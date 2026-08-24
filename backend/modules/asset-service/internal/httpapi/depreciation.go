package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/asset-service/internal/financeclient"
	"github.com/enterprise-digital-platform/asset-service/internal/model"
)

const depreciationRunColumns = `id, company_id, period, status, asset_count, total_amount, journal_entry_id, posted_at, notes, created_at, updated_at`

func scanDepreciationRun(row pgx.Row, run *model.DepreciationRun) error {
	return row.Scan(&run.ID, &run.CompanyID, &run.Period, &run.Status, &run.AssetCount, &run.TotalAmount,
		&run.JournalEntryID, &run.PostedAt, &run.Notes, &run.CreatedAt, &run.UpdatedAt)
}

// depreciationEntryView adalah entry berikut identitas asetnya. Entry sendiri
// hanya menyimpan asset_id; tanpa kode & nama, layar rincian penyusutan harus
// memanggil daftar aset lagi hanya untuk bisa dibaca manusia.
type depreciationEntryView struct {
	model.DepreciationEntry
	AssetCode string `json:"asset_code"`
	AssetName string `json:"asset_name"`
}

type depreciationRunDetail struct {
	model.DepreciationRun
	Entries []depreciationEntryView `json:"entries"`
}

// monthlyDepreciation menghitung penyusutan SATU BULAN untuk satu aset.
//
//	STRAIGHT_LINE     : (harga perolehan - nilai residu) / umur manfaat (bulan)
//	DECLINING_BALANCE : nilai buku x (2 / umur manfaat) -- saldo menurun ganda
//
// Dua hal yang disengaja:
//
//   - Tidak ada proporsi hari. Aset yang mulai disusutkan tanggal berapa pun
//     dalam sebuah bulan dikenai satu bulan penuh. Ini konvensi yang lazim
//     dipakai (dan yang membuat angkanya bisa dicocokkan dengan tabel
//     penyusutan manual); proporsi harian bisa ditambahkan nanti, tapi
//     setengah-setengah lebih buruk daripada konsisten.
//   - Bulan terakhir dipotong supaya nilai buku berhenti tepat di nilai residu.
//     Tanpa itu, saldo menurun tidak pernah mencapai nol dan garis lurus
//     melewatinya karena pembulatan.
func monthlyDepreciation(method string, cost, salvage, accumulated float64, usefulLifeMonths int) float64 {
	bookValue := cost - accumulated
	depreciable := bookValue - salvage
	if depreciable <= 0 || usefulLifeMonths <= 0 {
		return 0
	}

	var amount float64
	switch method {
	case "DECLINING_BALANCE":
		amount = bookValue * (2 / float64(usefulLifeMonths))
	default: // STRAIGHT_LINE
		amount = (cost - salvage) / float64(usefulLifeMonths)
	}
	if amount > depreciable {
		amount = depreciable
	}
	return round2(amount)
}

// periodEnd mengembalikan hari terakhir bulan 'YYYY-MM'.
func periodEnd(period string) (time.Time, error) {
	start, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Time{}, err
	}
	return start.AddDate(0, 1, -1), nil
}

func (h *Handler) listDepreciationRuns(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + depreciationRunColumns + ` FROM depreciation_runs WHERE company_id = $1`
	args := []any{companyID}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	query += ` ORDER BY period DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data penyusutan")
		return
	}
	defer rows.Close()

	runs := []model.DepreciationRun{}
	for rows.Next() {
		var run model.DepreciationRun
		if err := scanDepreciationRun(rows, &run); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data penyusutan")
			return
		}
		runs = append(runs, run)
	}
	writeJSON(w, http.StatusOK, runs)
}

type createDepreciationRunRequest struct {
	CompanyID string `json:"company_id"`
	Period    string `json:"period"`
	Notes     string `json:"notes"`
}

// createDepreciationRun menghitung penyusutan seluruh aset company untuk satu
// periode dan menyimpannya sebagai DRAFT. Belum menyentuh buku besar, dan belum
// menaikkan akumulasi penyusutan aset -- keduanya baru terjadi saat diposting.
//
// Dua penolakan yang menjaga urutan:
//
//   - masih ada run DRAFT (periode mana pun): perhitungan berikutnya akan
//     memakai akumulasi yang belum termasuk run itu, jadi periode berikutnya
//     akan menyusutkan angka yang sama dua kali.
//   - sudah ada run POSTED untuk periode ini atau yang lebih baru: akumulasi
//     penyusutan adalah angka berjalan, dan menyisipkan periode lama di
//     belakang periode yang sudah diposting membuat nilai buku tiap aset
//     tidak lagi cocok dengan urutan entry-nya.
func (h *Handler) createDepreciationRun(w http.ResponseWriter, r *http.Request) {
	var req createDepreciationRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.Period == "" {
		writeError(w, http.StatusBadRequest, "company_id dan period wajib diisi")
		return
	}
	end, err := periodEnd(req.Period)
	if err != nil {
		writeError(w, http.StatusBadRequest, "period harus format YYYY-MM")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var draftPeriod *string
	if err := tx.QueryRow(ctx,
		`SELECT period FROM depreciation_runs WHERE company_id = $1 AND status = 'DRAFT' ORDER BY period ASC LIMIT 1`,
		req.CompanyID).Scan(&draftPeriod); err != nil && err != pgx.ErrNoRows {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa run penyusutan")
		return
	}
	if draftPeriod != nil {
		writeError(w, http.StatusConflict, "Masih ada perhitungan penyusutan berstatus DRAFT (periode "+*draftPeriod+"); posting atau hapus dulu")
		return
	}

	var latestPosted *string
	if err := tx.QueryRow(ctx,
		`SELECT period FROM depreciation_runs WHERE company_id = $1 AND status = 'POSTED' ORDER BY period DESC LIMIT 1`,
		req.CompanyID).Scan(&latestPosted); err != nil && err != pgx.ErrNoRows {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa run penyusutan")
		return
	}
	if latestPosted != nil && *latestPosted >= req.Period {
		writeError(w, http.StatusConflict, "Periode "+*latestPosted+" sudah diposting; penyusutan tidak bisa dihitung mundur")
		return
	}

	rows, err := tx.Query(ctx, `
		SELECT id, acquisition_cost, salvage_value, accumulated_depreciation, useful_life_months, depreciation_method
		FROM assets
		WHERE company_id = $1
		  AND status <> 'DISPOSED'
		  AND useful_life_months IS NOT NULL
		  AND depreciation_start_date IS NOT NULL
		  AND depreciation_start_date <= $2
		  AND acquisition_cost - accumulated_depreciation > salvage_value
		ORDER BY asset_code ASC`, req.CompanyID, end)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat aset yang disusutkan")
		return
	}

	type calculated struct {
		assetID           string
		method            string
		amount            float64
		bookValueBefore   float64
		bookValueAfterVal float64
	}
	calcs := []calculated{}
	var total float64
	for rows.Next() {
		var (
			id          string
			cost        float64
			salvage     float64
			accumulated float64
			life        int
			method      string
		)
		if err := rows.Scan(&id, &cost, &salvage, &accumulated, &life, &method); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "Gagal membaca data aset")
			return
		}
		amount := monthlyDepreciation(method, cost, salvage, accumulated, life)
		if amount <= 0 {
			continue
		}
		before := round2(cost - accumulated)
		calcs = append(calcs, calculated{
			assetID: id, method: method, amount: amount,
			bookValueBefore: before, bookValueAfterVal: round2(before - amount),
		})
		total = round2(total + amount)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca data aset")
		return
	}
	if len(calcs) == 0 {
		writeError(w, http.StatusConflict, "Tidak ada aset yang perlu disusutkan pada periode ini")
		return
	}

	var run model.DepreciationRun
	err = scanDepreciationRun(tx.QueryRow(ctx, `
		INSERT INTO depreciation_runs (company_id, period, asset_count, total_amount, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+depreciationRunColumns,
		req.CompanyID, req.Period, len(calcs), total, req.Notes), &run)
	if err != nil {
		if isDuplicateKey(err) {
			writeError(w, http.StatusConflict, "Penyusutan periode ini sudah pernah dihitung")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat perhitungan penyusutan")
		return
	}

	for _, c := range calcs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO depreciation_entries (run_id, asset_id, method, amount, book_value_before, book_value_after)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			run.ID, c.assetID, c.method, c.amount, c.bookValueBefore, c.bookValueAfterVal); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal menyimpan rincian penyusutan")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perhitungan penyusutan")
		return
	}

	entries, err := h.fetchDepreciationEntries(ctx, run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat rincian penyusutan")
		return
	}

	h.events.Publish("asset.depreciation.calculated", newAuditEvent("asset.depreciation.calculated", actorFromHeader(r), &run.CompanyID, "create", "depreciation_run", run.ID, run))
	writeJSON(w, http.StatusCreated, depreciationRunDetail{DepreciationRun: run, Entries: entries})
}

func (h *Handler) fetchDepreciationEntries(ctx context.Context, runID string) ([]depreciationEntryView, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT e.id, e.run_id, e.asset_id, e.method, e.amount, e.book_value_before, e.book_value_after, e.created_at,
		       a.asset_code, a.name
		FROM depreciation_entries e
		JOIN assets a ON a.id = e.asset_id
		WHERE e.run_id = $1
		ORDER BY a.asset_code ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []depreciationEntryView{}
	for rows.Next() {
		var e depreciationEntryView
		if err := rows.Scan(&e.ID, &e.RunID, &e.AssetID, &e.Method, &e.Amount, &e.BookValueBefore, &e.BookValueAfter, &e.CreatedAt,
			&e.AssetCode, &e.AssetName); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (h *Handler) getDepreciationRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var run model.DepreciationRun
	err := scanDepreciationRun(h.pool.QueryRow(ctx, `SELECT `+depreciationRunColumns+` FROM depreciation_runs WHERE id = $1`, id), &run)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Perhitungan penyusutan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat perhitungan penyusutan")
		return
	}

	entries, err := h.fetchDepreciationEntries(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat rincian penyusutan")
		return
	}
	writeJSON(w, http.StatusOK, depreciationRunDetail{DepreciationRun: run, Entries: entries})
}

type postDepreciationRunRequest struct {
	ExpenseAccountID                 string `json:"expense_account_id"`
	AccumulatedDepreciationAccountID string `json:"accumulated_depreciation_account_id"`
}

// postDepreciationRun memposting satu jurnal untuk seluruh periode:
//
//	Debit  Beban Penyusutan          total
//	Kredit Akumulasi Penyusutan      total
//
// Satu jurnal per periode, bukan per aset: yang dibutuhkan buku besar adalah
// bebannya, sementara rinciannya sudah tersimpan sebagai depreciation_entries
// di sini dan bisa ditelusuri lewat reference_id jurnalnya.
//
// finance-service dipanggil DULU, akumulasi aset dinaikkan setelah jurnalnya
// berhasil -- sama seperti payroll dan penyelesaian work order. Kalau
// finance-service gagal, run tetap DRAFT dan bisa diposting ulang; kebalikannya
// (menaikkan akumulasi lalu gagal posting) meninggalkan nilai buku yang tidak
// punya jurnal pasangannya.
func (h *Handler) postDepreciationRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req postDepreciationRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.ExpenseAccountID == "" || req.AccumulatedDepreciationAccountID == "" {
		writeError(w, http.StatusBadRequest, "expense_account_id dan accumulated_depreciation_account_id wajib diisi")
		return
	}

	var run model.DepreciationRun
	err := scanDepreciationRun(h.pool.QueryRow(ctx, `SELECT `+depreciationRunColumns+` FROM depreciation_runs WHERE id = $1`, id), &run)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Perhitungan penyusutan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat perhitungan penyusutan")
		return
	}
	if run.Status != "DRAFT" {
		writeError(w, http.StatusConflict, "Perhitungan penyusutan ini sudah diposting")
		return
	}

	end, err := periodEnd(run.Period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Periode run tidak valid")
		return
	}

	entry, err := h.finance.CreateAndPostJournalEntry(headerValue(actor), financeclient.CreateJournalEntryRequest{
		CompanyID:     run.CompanyID,
		EntryDate:     end.Format("2006-01-02"),
		Description:   "Penyusutan aset " + run.Period,
		ReferenceType: "ASSET_DEPRECIATION",
		ReferenceID:   &run.ID,
		Lines: []financeclient.JournalLineInput{
			{AccountID: req.ExpenseAccountID, DebitAmount: run.TotalAmount, Description: "Beban Penyusutan " + run.Period},
			{AccountID: req.AccumulatedDepreciationAccountID, CreditAmount: run.TotalAmount, Description: "Akumulasi Penyusutan " + run.Period},
		},
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal memposting jurnal penyusutan ke finance-service: %v", err))
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Jurnal sudah diposting, tetapi gagal memulai transaksi lokal")
		return
	}
	defer tx.Rollback(ctx)

	err = scanDepreciationRun(tx.QueryRow(ctx, `
		UPDATE depreciation_runs SET status = 'POSTED', journal_entry_id = $1, posted_at = now(), updated_at = now()
		WHERE id = $2 AND status = 'DRAFT'
		RETURNING `+depreciationRunColumns, entry.ID, id), &run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Jurnal sudah diposting di finance-service, tetapi gagal memperbarui status perhitungan penyusutan")
		return
	}

	// Akumulasi penyusutan aset baru naik di sini -- setelah jurnalnya ada.
	if _, err := tx.Exec(ctx, `
		UPDATE assets a
		SET accumulated_depreciation = a.accumulated_depreciation + e.amount, updated_at = now()
		FROM depreciation_entries e
		WHERE e.run_id = $1 AND e.asset_id = a.id`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Jurnal sudah diposting di finance-service, tetapi gagal memperbarui akumulasi penyusutan aset")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Jurnal sudah diposting di finance-service, tetapi gagal menyimpan perubahan lokal")
		return
	}

	h.events.Publish("asset.depreciation.posted", newAuditEvent("asset.depreciation.posted", actor, &run.CompanyID, "update", "depreciation_run", run.ID, run))
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) deleteDepreciationRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Hanya DRAFT: run yang sudah diposting punya jurnal di buku besar, dan
	// menghapusnya di sini akan menyisakan jurnal tanpa asal-usul.
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM depreciation_runs WHERE id = $1 AND status = 'DRAFT'`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghapus perhitungan penyusutan")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "Perhitungan penyusutan tidak ditemukan atau sudah diposting")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
