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

	lines := make([]model.BOMLine, 0, len(req.Lines))
	for i, l := range req.Lines {
		// Baris formula menyimpan keduanya: percentage sebagaimana ditulis
		// orang, quantity_per_unit sebagai turunannya supaya kolom lama tidak
		// kosong. Yang dipakai menghitung kebutuhan work order tetap
		// percentage-nya -- lihat createWorkOrder.
		quantityPerUnit := l.QuantityPerUnit
		var percentage *float64
		if req.BOMType == model.BOMTypeBatch {
			quantityPerUnit = l.Percentage / 100
			percentage = &req.Lines[i].Percentage
		}
		var line model.BOMLine
		err := tx.QueryRow(ctx, `
			INSERT INTO bom_lines (bom_id, line_number, component_product_id, quantity_per_unit, percentage)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, bom_id, line_number, component_product_id, quantity_per_unit, percentage`,
			b.ID, i+1, l.ComponentProductID, quantityPerUnit, percentage,
		).Scan(&line.ID, &line.BOMID, &line.LineNumber, &line.ComponentProductID, &line.QuantityPerUnit, &line.Percentage)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris komponen BOM")
			return
		}
		lines = append(lines, line)
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

type updateBOMRequest struct {
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
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

	var b model.BillOfMaterial
	err := scanBOM(h.pool.QueryRow(r.Context(), `
		UPDATE bill_of_materials SET name = $1, is_active = $2, updated_at = now()
		WHERE id = $3
		RETURNING `+bomColumns,
		req.Name, req.IsActive, id,
	), &b)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "BOM tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui BOM")
		return
	}

	h.events.Publish("production.bom.updated", newAuditEvent("production.bom.updated", actorFromHeader(r), &b.CompanyID, "update", "bom", b.ID, b))
	writeJSON(w, http.StatusOK, b)
}
