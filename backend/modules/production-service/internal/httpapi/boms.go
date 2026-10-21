package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/production-service/internal/model"
)

const bomColumns = `id, company_id, branch_id, bom_code, name, product_id, bom_type, batch_size, is_active, created_at, updated_at`

func scanBOM(row pgx.Row, b *model.BillOfMaterial) error {
	return row.Scan(&b.ID, &b.CompanyID, &b.BranchID, &b.BOMCode, &b.Name, &b.ProductID, &b.BOMType, &b.BatchSize, &b.IsActive, &b.CreatedAt, &b.UpdatedAt)
}

func (h *Handler) listBOMs(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + bomColumns + ` FROM bill_of_materials WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY bom_code ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data BOM")
		return
	}
	defer rows.Close()

	boms := []model.BillOfMaterial{}
	for rows.Next() {
		var b model.BillOfMaterial
		if err := scanBOM(rows, &b); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data BOM")
			return
		}
		boms = append(boms, b)
	}
	writeJSON(w, http.StatusOK, boms)
}

type bomLineInput struct {
	ComponentProductID string  `json:"component_product_id"`
	QuantityPerUnit    float64 `json:"quantity_per_unit"`
	Percentage         float64 `json:"percentage"`
}

type createBOMRequest struct {
	CompanyID string         `json:"company_id"`
	BranchID  *string        `json:"branch_id"`
	BOMCode   string         `json:"bom_code"`
	Name      string         `json:"name"`
	ProductID string         `json:"product_id"`
	BOMType   string         `json:"bom_type"`
	BatchSize float64        `json:"batch_size"`
	Lines     []bomLineInput `json:"lines"`
}

// bomWithLines menambahkan dua angka turunan untuk BOM BATCH, supaya UI tidak
// menghitungnya sendiri dengan rumus versinya: TotalPercentage adalah jumlah
// persentase seluruh baris, dan YieldPercent berapa persen dari input yang
// benar-benar menjadi produk (100 / total x 100). Formula 105% berarti yield
// 95,2% -- susut prosesnya terbaca sebagai angka, bukan tersembunyi di
// selisih. Keduanya nil untuk BOM UNIT, di mana persentase tidak punya arti.
type bomWithLines struct {
	model.BillOfMaterial
	Lines           []model.BOMLine `json:"lines"`
	TotalPercentage *float64        `json:"total_percentage"`
	YieldPercent    *float64        `json:"yield_percent"`
}

func newBOMWithLines(b model.BillOfMaterial, lines []model.BOMLine) bomWithLines {
	out := bomWithLines{BillOfMaterial: b, Lines: lines}
	if b.BOMType != model.BOMTypeBatch {
		return out
	}
	total := 0.0
	for _, l := range lines {
		if l.Percentage != nil {
			total += *l.Percentage
		}
	}
	out.TotalPercentage = &total
	if total > 0 {
		yield := 100 / total * 100
		out.YieldPercent = &yield
	}
	return out
}

// minLinePercentage: quantity_per_unit turunannya (percentage / 100) disimpan
// di kolom NUMERIC(15,4), jadi persentase di bawah 0,01% membulat menjadi nol
// dan melanggar CHECK quantity_per_unit > 0. Ditolak di sini dengan pesan yang
// menyebut angkanya, bukan dibiarkan jatuh sebagai galat database.
const minLinePercentage = 0.01

// validateBOMLines memisahkan dua jenis resep. BOM UNIT: tiap baris wajib
// quantity_per_unit > 0, dan persentase tidak boleh ikut dikirim (kalau ikut,
// pengirimnya sedang salah mengira tipe BOM-nya). Formula BATCH kebalikannya,
// ditambah satu aturan lintas baris: jumlah persentase wajib >= 100% --
// neraca massa, satu batch penuh tidak bisa keluar dari input yang kurang dari
// itu. Kelebihan di atas 100% adalah susut proses dan memang sah.
func validateBOMLines(bomType string, batchSize float64, lines []bomLineInput) string {
	if bomType == model.BOMTypeUnit {
		if batchSize != 0 {
			return "batch_size hanya berlaku untuk bom_type BATCH"
		}
		for _, l := range lines {
			if l.ComponentProductID == "" || l.QuantityPerUnit <= 0 {
				return "Setiap komponen wajib punya component_product_id dan quantity_per_unit > 0"
			}
			if l.Percentage != 0 {
				return "percentage hanya berlaku untuk bom_type BATCH"
			}
		}
		return ""
	}

	if batchSize <= 0 {
		return "batch_size > 0 wajib diisi untuk bom_type BATCH"
	}
	total := 0.0
	for _, l := range lines {
		if l.ComponentProductID == "" || l.Percentage <= 0 {
			return "Setiap komponen formula wajib punya component_product_id dan percentage > 0"
		}
		if l.Percentage < minLinePercentage {
			return fmt.Sprintf("percentage terkecil yang bisa disimpan adalah %g%%", minLinePercentage)
		}
		if l.QuantityPerUnit != 0 {
			return "quantity_per_unit hanya berlaku untuk bom_type UNIT; formula memakai percentage"
		}
		total += l.Percentage
	}
	// Toleransi 0,0001: sisa pembulatan pecahan biner, bukan formula yang
	// benar-benar kurang dari satu batch.
	if total < 100-0.0001 {
		return fmt.Sprintf("Jumlah percentage seluruh komponen %.4f%%, kurang dari 100%% -- satu batch %g tidak bisa keluar dari input yang lebih sedikit", total, batchSize)
	}
	return ""
}

// insertBOMLines menulis baris komponen sebuah BOM. Baris formula menyimpan
// keduanya: percentage sebagaimana ditulis orang, quantity_per_unit sebagai
// turunannya supaya kolom lama tidak kosong. Yang dipakai menghitung kebutuhan
// work order tetap percentage-nya -- lihat createWorkOrder.
func insertBOMLines(ctx context.Context, tx pgx.Tx, bomID, bomType string, in []bomLineInput) ([]model.BOMLine, error) {
	lines := make([]model.BOMLine, 0, len(in))
	for i, l := range in {
		quantityPerUnit := l.QuantityPerUnit
		var percentage *float64
		if bomType == model.BOMTypeBatch {
			quantityPerUnit = l.Percentage / 100
			p := l.Percentage
			percentage = &p
		}
		var line model.BOMLine
		err := tx.QueryRow(ctx, `
			INSERT INTO bom_lines (bom_id, line_number, component_product_id, quantity_per_unit, percentage)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, bom_id, line_number, component_product_id, quantity_per_unit, percentage`,
			bomID, i+1, l.ComponentProductID, quantityPerUnit, percentage,
		).Scan(&line.ID, &line.BOMID, &line.LineNumber, &line.ComponentProductID, &line.QuantityPerUnit, &line.Percentage)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func (h *Handler) createBOM(w http.ResponseWriter, r *http.Request) {
	var req createBOMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.BOMCode = strings.TrimSpace(req.BOMCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.BOMCode == "" || req.Name == "" || req.ProductID == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, bom_code, name, product_id, dan minimal 1 komponen wajib diisi")
		return
	}
	// bom_type yang tidak dikirim berarti BOM diskrit: seluruh pemanggil yang
	// sudah ada (dan seluruh BOM yang sudah tersimpan) bertipe UNIT.
	if req.BOMType == "" {
		req.BOMType = model.BOMTypeUnit
	}
	if req.BOMType != model.BOMTypeUnit && req.BOMType != model.BOMTypeBatch {
		writeError(w, http.StatusBadRequest, "bom_type harus UNIT atau BATCH")
		return
	}
	if msg := validateBOMLines(req.BOMType, req.BatchSize, req.Lines); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	var batchSize *float64
	if req.BOMType == model.BOMTypeBatch {
		batchSize = &req.BatchSize
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var b model.BillOfMaterial
	err = scanBOM(tx.QueryRow(ctx, `
		INSERT INTO bill_of_materials (company_id, branch_id, bom_code, name, product_id, bom_type, batch_size)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+bomColumns,
		req.CompanyID, req.BranchID, req.BOMCode, req.Name, req.ProductID, req.BOMType, batchSize,
	), &b)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode BOM sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat BOM")
		return
	}

	lines, err := insertBOMLines(ctx, tx, b.ID, req.BOMType, req.Lines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat baris komponen BOM")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan BOM")
		return
	}

	h.events.Publish("production.bom.created", newAuditEvent("production.bom.created", actorFromHeader(r), &b.CompanyID, "create", "bom", b.ID, b))
	writeJSON(w, http.StatusCreated, newBOMWithLines(b, lines))
}

func (h *Handler) fetchBOMLines(ctx context.Context, bomID string) ([]model.BOMLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, bom_id, line_number, component_product_id, quantity_per_unit, percentage
		FROM bom_lines WHERE bom_id = $1 ORDER BY line_number ASC`, bomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.BOMLine{}
	for rows.Next() {
		var l model.BOMLine
		if err := rows.Scan(&l.ID, &l.BOMID, &l.LineNumber, &l.ComponentProductID, &l.QuantityPerUnit, &l.Percentage); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (h *Handler) getBOM(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var b model.BillOfMaterial
	err := scanBOM(h.pool.QueryRow(ctx, `SELECT `+bomColumns+` FROM bill_of_materials WHERE id = $1`, id), &b)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "BOM tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat BOM")
		return
	}

	lines, err := h.fetchBOMLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris komponen BOM")
		return
	}
	writeJSON(w, http.StatusOK, newBOMWithLines(b, lines))
}

// updateBOMRequest: name dan is_active seperti semula (is_active yang tidak
// dikirim berarti false -- perilaku lama, dipertahankan). lines, kalau dikirim,
// MENGGANTI seluruh komponen BOM; batch_size, kalau dikirim, mengganti ukuran
// batch formula. Tipe BOM (UNIT/BATCH) tidak bisa diubah: mengubahnya berarti
// resep yang artinya lain sama sekali, bukan penyuntingan -- buat BOM baru.
// Work order yang sudah ada tidak terpengaruh: kebutuhan komponennya sudah
// di-snapshot ke work_order_lines saat dibuat.
type updateBOMRequest struct {
	Name      string          `json:"name"`
	IsActive  bool            `json:"is_active"`
	BatchSize *float64        `json:"batch_size"`
	Lines     *[]bomLineInput `json:"lines"`
}

func (h *Handler) updateBOM(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateBOMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}
	if req.Lines != nil && len(*req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "lines tidak boleh kosong; BOM minimal punya 1 komponen")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	// Kunci baris BOM supaya dua penyuntingan bersamaan tidak saling menimpa
	// komponen di tengah jalan.
	var current model.BillOfMaterial
	err = scanBOM(tx.QueryRow(ctx, `SELECT `+bomColumns+` FROM bill_of_materials WHERE id = $1 FOR UPDATE`, id), &current)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "BOM tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat BOM")
		return
	}

	batchSize := current.BatchSize
	if req.BatchSize != nil {
		if current.BOMType != model.BOMTypeBatch {
			writeError(w, http.StatusBadRequest, "batch_size hanya berlaku untuk bom_type BATCH")
			return
		}
		if *req.BatchSize <= 0 {
			writeError(w, http.StatusBadRequest, "batch_size > 0 wajib diisi untuk bom_type BATCH")
			return
		}
		batchSize = req.BatchSize
	}
	if req.Lines != nil {
		size := 0.0
		if batchSize != nil {
			size = *batchSize
		}
		if msg := validateBOMLines(current.BOMType, size, *req.Lines); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}

	var b model.BillOfMaterial
	err = scanBOM(tx.QueryRow(ctx, `
		UPDATE bill_of_materials SET name = $1, is_active = $2, batch_size = $3, updated_at = now()
		WHERE id = $4
		RETURNING `+bomColumns,
		req.Name, req.IsActive, batchSize, id,
	), &b)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui BOM")
		return
	}

	var lines []model.BOMLine
	if req.Lines != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM bom_lines WHERE bom_id = $1`, id); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mengganti komponen BOM")
			return
		}
		if lines, err = insertBOMLines(ctx, tx, id, b.BOMType, *req.Lines); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mengganti komponen BOM")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan BOM")
		return
	}

	h.events.Publish("production.bom.updated", newAuditEvent("production.bom.updated", actorFromHeader(r), &b.CompanyID, "update", "bom", b.ID, b))
	if req.Lines == nil {
		writeJSON(w, http.StatusOK, b)
		return
	}
	writeJSON(w, http.StatusOK, newBOMWithLines(b, lines))
}
