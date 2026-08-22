package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/hr-service/internal/model"
)

// KPI karyawan: indikator (master) + penilaian per karyawan per periode.
//
// Aturan penilaian yang dipakai di sini:
//
//	achievement = actual / target * 100, dibatasi maxAchievement
//	score       = achievement * weight / 100
//	total_score = jumlah seluruh score
//
// Pembatasan achievement itu disengaja: tanpa batas, satu indikator yang
// tercapai 500% bisa menutupi seluruh indikator lain yang gagal, dan nilai
// akhirnya berhenti berarti. Batasnya di atas 100 supaya pencapaian melebihi
// target tetap dihargai, tapi tidak tanpa batas.
const maxAchievement = 150.0

var periodPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

var kpiRatings = []struct {
	minScore float64
	label    string
}{
	{90, "SANGAT BAIK"},
	{75, "BAIK"},
	{60, "CUKUP"},
	{0, "PERLU PERBAIKAN"},
}

func ratingFor(totalScore float64) string {
	for _, r := range kpiRatings {
		if totalScore >= r.minScore {
			return r.label
		}
	}
	return kpiRatings[len(kpiRatings)-1].label
}

// ---------- Indikator ----------

const kpiIndicatorColumns = `id, company_id, code, name, COALESCE(description, ''), unit, target_value, weight,
	is_active, created_by, created_at, updated_at`

func scanIndicator(row pgx.Row, i *model.KPIIndicator) error {
	return row.Scan(&i.ID, &i.CompanyID, &i.Code, &i.Name, &i.Description, &i.Unit, &i.TargetValue,
		&i.Weight, &i.IsActive, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt)
}

type kpiIndicatorPayload struct {
	CompanyID   string   `json:"company_id"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Unit        string   `json:"unit"`
	TargetValue float64  `json:"target_value"`
	Weight      float64  `json:"weight"`
	IsActive    *bool    `json:"is_active"`
	_           struct{} // payload ditutup supaya penambahan field kelak terlihat di diff
}

func (h *Handler) listKPIIndicators(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	// activeOnly default false: halaman master perlu melihat yang nonaktif juga.
	activeOnly := r.URL.Query().Get("active_only") == "true"

	rows, err := h.pool.Query(r.Context(), `
		SELECT `+kpiIndicatorColumns+`
		FROM kpi_indicators
		WHERE company_id = $1 AND ($2 = false OR is_active = true)
		ORDER BY code ASC`, companyID, activeOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat indikator KPI")
		return
	}
	defer rows.Close()

	indicators := []model.KPIIndicator{}
	for rows.Next() {
		var i model.KPIIndicator
		if err := scanIndicator(rows, &i); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca indikator KPI")
			return
		}
		indicators = append(indicators, i)
	}
	writeJSON(w, http.StatusOK, indicators)
}

func (h *Handler) createKPIIndicator(w http.ResponseWriter, r *http.Request) {
	var req kpiIndicatorPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if msg := validateIndicator(&req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var i model.KPIIndicator
	err := scanIndicator(h.pool.QueryRow(r.Context(), `
		INSERT INTO kpi_indicators (company_id, code, name, description, unit, target_value, weight, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+kpiIndicatorColumns,
		req.CompanyID, req.Code, req.Name, nullIfEmpty(req.Description), req.Unit,
		req.TargetValue, req.Weight, actorFromHeader(r)), &i)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Code indikator sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan indikator KPI")
		return
	}

	h.events.Publish("hr.kpi_indicator.created", newAuditEvent("hr.kpi_indicator.created", actorFromHeader(r), &i.CompanyID, "create", "kpi_indicator", i.ID, i))
	writeJSON(w, http.StatusCreated, i)
}

// updateKPIIndicator mengubah master indikator. Penilaian yang SUDAH dibuat
// tidak ikut berubah -- rinciannya menyimpan salinan nama/bobot/target sendiri
// (lihat komentar di 005_kpi.sql).
func (h *Handler) updateKPIIndicator(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req kpiIndicatorPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	// company_id & code tidak ikut diubah: keduanya identitas indikator.
	req.CompanyID = "-"
	req.Code = "-"
	if msg := validateIndicator(&req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var i model.KPIIndicator
	err := scanIndicator(h.pool.QueryRow(r.Context(), `
		UPDATE kpi_indicators
		SET name = $1, description = $2, unit = $3, target_value = $4, weight = $5, is_active = $6, updated_at = now()
		WHERE id = $7
		RETURNING `+kpiIndicatorColumns,
		req.Name, nullIfEmpty(req.Description), req.Unit, req.TargetValue, req.Weight, isActive, id), &i)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Indikator KPI tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui indikator KPI")
		return
	}

	h.events.Publish("hr.kpi_indicator.updated", newAuditEvent("hr.kpi_indicator.updated", actorFromHeader(r), &i.CompanyID, "update", "kpi_indicator", i.ID, i))
	writeJSON(w, http.StatusOK, i)
}

// deleteKPIIndicator hanya boleh untuk indikator yang belum pernah dipakai di
// penilaian mana pun. Yang sudah pernah dipakai dinonaktifkan saja (is_active
// = false) supaya penilaian lama tetap bisa dibaca utuh -- itu juga yang
// dijaga ON DELETE RESTRICT di kpi_review_items.
func (h *Handler) deleteKPIIndicator(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var used bool
	if err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM kpi_review_items WHERE indicator_id = $1)`, id).Scan(&used); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa pemakaian indikator")
		return
	}
	if used {
		writeError(w, http.StatusConflict, "Indikator ini sudah dipakai di penilaian; nonaktifkan saja supaya penilaian lama tetap utuh")
		return
	}

	var companyID string
	err := h.pool.QueryRow(ctx, `DELETE FROM kpi_indicators WHERE id = $1 RETURNING company_id`, id).Scan(&companyID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Indikator KPI tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghapus indikator KPI")
		return
	}

	h.events.Publish("hr.kpi_indicator.deleted", newAuditEvent("hr.kpi_indicator.deleted", actorFromHeader(r), &companyID, "delete", "kpi_indicator", id, nil))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func validateIndicator(req *kpiIndicatorPayload) string {
	req.Code = strings.ToUpper(strings.Join(strings.Fields(req.Code), "_"))
	req.Name = strings.TrimSpace(req.Name)
	req.Unit = strings.TrimSpace(req.Unit)
	if req.Unit == "" {
		req.Unit = "poin"
	}
	if req.CompanyID == "" || req.Code == "" || req.Name == "" {
		return "company_id, code, dan name wajib diisi"
	}
	if req.TargetValue <= 0 {
		return "target_value harus lebih dari 0"
	}
	if req.Weight <= 0 || req.Weight > 100 {
		return "weight harus di antara 0 dan 100"
	}
	return ""
}

// ---------- Penilaian ----------

const kpiReviewColumns = `id, company_id, branch_id, employee_id, employee_name, period, status,
	total_score, rating, COALESCE(notes, ''), COALESCE(rejection_reason, ''), submitted_at, decided_at,
	decided_by, created_by, created_at, updated_at`

func scanReview(row pgx.Row, v *model.KPIReview) error {
	return row.Scan(&v.ID, &v.CompanyID, &v.BranchID, &v.EmployeeID, &v.EmployeeName, &v.Period, &v.Status,
		&v.TotalScore, &v.Rating, &v.Notes, &v.RejectionReason, &v.SubmittedAt, &v.DecidedAt,
		&v.DecidedBy, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
}

type kpiReviewPayload struct {
	CompanyID  string  `json:"company_id"`
	BranchID   *string `json:"branch_id"`
	EmployeeID string  `json:"employee_id"`
	Period     string  `json:"period"`
	Notes      string  `json:"notes"`
}

func (h *Handler) listKPIReviews(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	args := []any{companyID}
	query := `SELECT ` + kpiReviewColumns + ` FROM kpi_reviews WHERE company_id = $1`

	if period := r.URL.Query().Get("period"); period != "" {
		if !periodPattern.MatchString(period) {
			writeError(w, http.StatusBadRequest, "period harus berformat YYYY-MM")
			return
		}
		args = append(args, period)
		query += ` AND period = $` + strconv.Itoa(len(args))
	}
	if employeeID := r.URL.Query().Get("employee_id"); employeeID != "" {
		args = append(args, employeeID)
		query += ` AND employee_id = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		// NULL-inclusive, pola yang sama dengan seluruh filter branch di platform ini.
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY period DESC, employee_name ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat penilaian KPI")
		return
	}
	defer rows.Close()

	reviews := []model.KPIReview{}
	for rows.Next() {
		var v model.KPIReview
		if err := scanReview(rows, &v); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca penilaian KPI")
			return
		}
		reviews = append(reviews, v)
	}
	writeJSON(w, http.StatusOK, reviews)
}

// createKPIReview membuat penilaian DRAFT sekaligus MENYALIN seluruh indikator
// aktif menjadi rinciannya. Menyalin di sini (bukan saat dibaca) yang membuat
// penilaian tetap utuh walau master indikatornya diubah setelahnya.
func (h *Handler) createKPIReview(w http.ResponseWriter, r *http.Request) {
	var req kpiReviewPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.EmployeeID == "" || req.Period == "" {
		writeError(w, http.StatusBadRequest, "company_id, employee_id, dan period wajib diisi")
		return
	}
	if !periodPattern.MatchString(req.Period) {
		writeError(w, http.StatusBadRequest, "period harus berformat YYYY-MM")
		return
	}

	ctx := r.Context()
	emp, errMsg, status := h.activeEmployee(ctx, req.EmployeeID, req.CompanyID)
	if errMsg != "" {
		writeError(w, status, errMsg)
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var v model.KPIReview
	err = scanReview(tx.QueryRow(ctx, `
		INSERT INTO kpi_reviews (company_id, branch_id, employee_id, employee_name, period, notes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+kpiReviewColumns,
		req.CompanyID, req.BranchID, req.EmployeeID, employeeFullName(emp), req.Period,
		nullIfEmpty(req.Notes), actorFromHeader(r)), &v)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Karyawan ini sudah punya penilaian untuk periode tersebut")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan penilaian KPI")
		return
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO kpi_review_items (review_id, indicator_id, indicator_name, unit, target_value, weight)
		SELECT $1, id, name, unit, target_value, weight
		FROM kpi_indicators
		WHERE company_id = $2 AND is_active = true`, v.ID, req.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyalin indikator ke penilaian")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusBadRequest, "Belum ada indikator KPI aktif di company ini")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan penilaian KPI")
		return
	}

	h.events.Publish("hr.kpi_review.created", newAuditEvent("hr.kpi_review.created", actorFromHeader(r), &v.CompanyID, "create", "kpi_review", v.ID, v))
	writeJSON(w, http.StatusCreated, v)
}

type kpiReviewDetail struct {
	model.KPIReview
	Items []model.KPIReviewItem `json:"items"`
	// TotalWeight memudahkan UI menampilkan peringatan "bobot belum 100%"
	// tanpa menjumlahkan sendiri.
	TotalWeight float64 `json:"total_weight"`
}

func (h *Handler) getKPIReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var v model.KPIReview
	err := scanReview(h.pool.QueryRow(ctx, `SELECT `+kpiReviewColumns+` FROM kpi_reviews WHERE id = $1`, id), &v)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Penilaian KPI tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat penilaian KPI")
		return
	}

	items, totalWeight, err := h.kpiItems(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat rincian penilaian")
		return
	}
	writeJSON(w, http.StatusOK, kpiReviewDetail{KPIReview: v, Items: items, TotalWeight: totalWeight})
}

func (h *Handler) kpiItems(ctx context.Context, reviewID string) ([]model.KPIReviewItem, float64, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, review_id, indicator_id, indicator_name, unit, target_value, weight,
		       actual_value, achievement, score, COALESCE(note, '')
		FROM kpi_review_items WHERE review_id = $1 ORDER BY indicator_name ASC`, reviewID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []model.KPIReviewItem{}
	var totalWeight float64
	for rows.Next() {
		var it model.KPIReviewItem
		if err := rows.Scan(&it.ID, &it.ReviewID, &it.IndicatorID, &it.IndicatorName, &it.Unit,
			&it.TargetValue, &it.Weight, &it.ActualValue, &it.Achievement, &it.Score, &it.Note); err != nil {
			return nil, 0, err
		}
		totalWeight += it.Weight
		items = append(items, it)
	}
	return items, round2(totalWeight), rows.Err()
}

type kpiScorePayload struct {
	Items []struct {
		IndicatorID string  `json:"indicator_id"`
		ActualValue float64 `json:"actual_value"`
		Note        string  `json:"note"`
	} `json:"items"`
	Notes string `json:"notes"`
}

// putKPIScores mengisi realisasi tiap indikator dan menghitung ulang nilainya.
// Hanya boleh saat DRAFT: begitu diajukan, angkanya dikunci -- kalau masih bisa
// diubah, penyetuju bisa menyetujui nilai yang berbeda dari yang dia baca
// (pagar yang sama dengan cuti & lembur).
func (h *Handler) putKPIScores(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req kpiScorePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var v model.KPIReview
	err = scanReview(tx.QueryRow(ctx, `SELECT `+kpiReviewColumns+` FROM kpi_reviews WHERE id = $1 FOR UPDATE`, id), &v)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Penilaian KPI tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat penilaian KPI")
		return
	}
	if v.Status != "DRAFT" {
		writeError(w, http.StatusConflict, "Hanya penilaian DRAFT yang bisa diisi (status sekarang: "+v.Status+")")
		return
	}

	for _, item := range req.Items {
		if item.ActualValue < 0 {
			writeError(w, http.StatusBadRequest, "actual_value tidak boleh negatif")
			return
		}
		tag, err := tx.Exec(ctx, `
			UPDATE kpi_review_items
			SET actual_value = $1,
			    achievement = LEAST(ROUND($1 / target_value * 100, 2), $4),
			    score = ROUND(LEAST(ROUND($1 / target_value * 100, 2), $4) * weight / 100, 2),
			    note = $2,
			    updated_at = now()
			WHERE review_id = $3 AND indicator_id = $5`,
			item.ActualValue, nullIfEmpty(item.Note), id, maxAchievement, item.IndicatorID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal menyimpan nilai indikator")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusBadRequest, "Ada indicator_id yang bukan bagian dari penilaian ini")
			return
		}
	}

	if err := recalculateKPITotal(ctx, tx, id, req.Notes); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghitung total nilai")
		return
	}

	err = scanReview(tx.QueryRow(ctx, `SELECT `+kpiReviewColumns+` FROM kpi_reviews WHERE id = $1`, id), &v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca penilaian KPI")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan penilaian KPI")
		return
	}

	h.events.Publish("hr.kpi_review.scored", newAuditEvent("hr.kpi_review.scored", actorFromHeader(r), &v.CompanyID, "update", "kpi_review", v.ID, v))
	writeJSON(w, http.StatusOK, v)
}

// recalculateKPITotal menjumlahkan score seluruh rincian dan menetapkan rating.
// Dijalankan di dalam transaksi pemanggilnya supaya total & rincian tidak
// pernah tersimpan setengah-setengah.
func recalculateKPITotal(ctx context.Context, tx pgx.Tx, reviewID, notes string) error {
	var total float64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(score), 0) FROM kpi_review_items WHERE review_id = $1`, reviewID).Scan(&total); err != nil {
		return err
	}
	total = round2(total)
	_, err := tx.Exec(ctx, `
		UPDATE kpi_reviews
		SET total_score = $1, rating = $2, notes = COALESCE(NULLIF($3, ''), notes), updated_at = now()
		WHERE id = $4`, total, ratingFor(total), notes, reviewID)
	return err
}

type kpiDecisionRequest struct {
	RejectionReason string `json:"rejection_reason"`
}

func (h *Handler) submitKPIReview(w http.ResponseWriter, r *http.Request) {
	h.transitionKPIReview(w, r, []string{"DRAFT", "REJECTED"}, "SUBMITTED", "hr.kpi_review.submitted", "Hanya penilaian DRAFT atau REJECTED yang bisa diajukan")
}

func (h *Handler) approveKPIReview(w http.ResponseWriter, r *http.Request) {
	h.transitionKPIReview(w, r, []string{"SUBMITTED"}, "APPROVED", "hr.kpi_review.approved", "Hanya penilaian SUBMITTED yang bisa disetujui")
}

func (h *Handler) rejectKPIReview(w http.ResponseWriter, r *http.Request) {
	h.transitionKPIReview(w, r, []string{"SUBMITTED"}, "REJECTED", "hr.kpi_review.rejected", "Hanya penilaian SUBMITTED yang bisa ditolak")
}

func (h *Handler) transitionKPIReview(w http.ResponseWriter, r *http.Request, from []string, to, eventType, notAllowedMsg string) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req kpiDecisionRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if to == "REJECTED" && strings.TrimSpace(req.RejectionReason) == "" {
		writeError(w, http.StatusBadRequest, "rejection_reason wajib diisi saat menolak penilaian")
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var v model.KPIReview
	err = scanReview(tx.QueryRow(ctx, `SELECT `+kpiReviewColumns+` FROM kpi_reviews WHERE id = $1 FOR UPDATE`, id), &v)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Penilaian KPI tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat penilaian KPI")
		return
	}
	if !slices.Contains(from, v.Status) {
		writeError(w, http.StatusConflict, notAllowedMsg+" (status sekarang: "+v.Status+")")
		return
	}

	// Bobot diperiksa saat PENGAJUAN, bukan saat indikator dibuat: bobot yang
	// belum genap 100% adalah keadaan wajar selama master indikator masih
	// disusun, tapi nilai akhir dari bobot yang tidak genap tidak bisa
	// dibandingkan antar karyawan.
	if to == "SUBMITTED" {
		var totalWeight float64
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(SUM(weight), 0) FROM kpi_review_items WHERE review_id = $1`, id).Scan(&totalWeight); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa bobot indikator")
			return
		}
		if round2(totalWeight) != 100 {
			writeError(w, http.StatusConflict,
				"Total bobot indikator pada penilaian ini "+strconv.FormatFloat(round2(totalWeight), 'f', -1, 64)+"%, harus tepat 100%")
			return
		}
	}

	setClause := `status = $1, updated_at = now()`
	switch to {
	case "SUBMITTED":
		setClause += `, submitted_at = now(), rejection_reason = NULL`
	case "APPROVED":
		setClause += `, decided_at = now(), decided_by = $3, rejection_reason = NULL`
	case "REJECTED":
		setClause += `, decided_at = now(), decided_by = $3, rejection_reason = $4`
	}

	args := []any{to, id}
	if to == "APPROVED" || to == "REJECTED" {
		args = append(args, actor)
	}
	if to == "REJECTED" {
		args = append(args, strings.TrimSpace(req.RejectionReason))
	}

	err = scanReview(tx.QueryRow(ctx, `UPDATE kpi_reviews SET `+setClause+` WHERE id = $2 RETURNING `+kpiReviewColumns, args...), &v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status penilaian")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perubahan penilaian")
		return
	}

	h.events.Publish(eventType, newAuditEvent(eventType, actor, &v.CompanyID, "update", "kpi_review", v.ID, v))
	writeJSON(w, http.StatusOK, v)
}
