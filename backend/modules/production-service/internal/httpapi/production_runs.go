package httpapi

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/production-service/internal/model"
)

const productionRunColumns = `id, company_id, branch_id, run_number, work_order_id, machine_id, shift_id, run_date, planned_minutes, quantity_good, quantity_reject, status, notes, created_at, updated_at`

var downtimeReasons = map[string]bool{
	"BREAKDOWN": true, "SETUP": true, "MATERIAL_SHORTAGE": true, "NO_OPERATOR": true,
	"ADJUSTMENT": true, "PLANNED_STOP": true, "OTHER": true,
}

func scanProductionRun(row pgx.Row, run *model.ProductionRun) error {
	return row.Scan(&run.ID, &run.CompanyID, &run.BranchID, &run.RunNumber, &run.WorkOrderID, &run.MachineID, &run.ShiftID,
		&run.RunDate, &run.PlannedMinutes, &run.QuantityGood, &run.QuantityReject, &run.Status, &run.Notes, &run.CreatedAt, &run.UpdatedAt)
}

// oeeMetrics adalah tiga faktor OEE beserta bahan mentahnya. Bahan mentahnya
// ikut dikirim (bukan hanya persentasenya) supaya angka yang mencurigakan bisa
// ditelusuri langsung dari layar tanpa membuka database.
type oeeMetrics struct {
	PlannedMinutes  int     `json:"planned_minutes"`
	DowntimeMinutes int     `json:"downtime_minutes"`
	RunTimeMinutes  int     `json:"run_time_minutes"`
	QuantityGood    float64 `json:"quantity_good"`
	QuantityReject  float64 `json:"quantity_reject"`
	Availability    float64 `json:"availability"`
	Performance     float64 `json:"performance"`
	Quality         float64 `json:"quality"`
	OEE             float64 `json:"oee"`
	// PerformanceCapped menandai Performance yang aslinya di atas 100% dan
	// dipotong ke 100%. Itu bukan mesin yang melampaui rancangannya, itu
	// ideal_cycle_time_minutes yang disetel terlalu lambat atau downtime yang
	// tidak dicatat -- dibiarkan lewat begitu saja, OEE-nya jadi bohong ke
	// atas dan tidak ada yang tahu penyebabnya.
	PerformanceCapped bool `json:"performance_capped"`
}

// computeOEE memakai definisi OEE yang standar:
//
//	Availability = run time / planned time
//	Performance  = (ideal cycle time * total unit) / run time
//	Quality      = unit bagus / total unit
//	OEE          = Availability * Performance * Quality
//
// planned time di sini adalah menit produktif shift (sudah dikurangi
// istirahat), bukan 24 jam: OEE mengukur mesin terhadap waktu yang memang
// dijadwalkan untuk berproduksi.
func computeOEE(plannedMinutes, downtimeMinutes int, good, reject, idealCycleTimeMinutes float64) oeeMetrics {
	return aggregateOEE(plannedMinutes, downtimeMinutes, good, reject, idealCycleTimeMinutes*(good+reject))
}

func round4(m oeeMetrics) oeeMetrics {
	r := func(v float64) float64 { return math.Round(v*10000) / 10000 }
	m.Availability, m.Performance, m.Quality, m.OEE = r(m.Availability), r(m.Performance), r(m.Quality), r(m.OEE)
	return m
}

type productionRunDetail struct {
	model.ProductionRun
	Machine         *model.Machine      `json:"machine,omitempty"`
	DowntimeLogs    []model.DowntimeLog `json:"downtime_logs"`
	DowntimeMinutes int                 `json:"downtime_minutes"`
	// OEE hanya diisi untuk run yang sudah CLOSED -- selama run masih berjalan
	// jumlah unitnya belum ada, dan menampilkan OEE 0% untuk run yang baru
	// dimulai lebih menyesatkan daripada tidak menampilkan apa pun.
	OEE *oeeMetrics `json:"oee,omitempty"`
}

func (h *Handler) listProductionRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	companyID := q.Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + productionRunColumns + ` FROM production_runs WHERE company_id = $1`
	args := []any{companyID}
	addFilter := func(value, clause string) {
		if value == "" {
			return
		}
		args = append(args, value)
		query += ` AND ` + clause + ` $` + strconv.Itoa(len(args))
	}
	if branchID := q.Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	addFilter(q.Get("work_order_id"), `work_order_id =`)
	addFilter(q.Get("machine_id"), `machine_id =`)
	addFilter(q.Get("status"), `status =`)
	addFilter(q.Get("from"), `run_date >=`)
	addFilter(q.Get("to"), `run_date <=`)
	query += ` ORDER BY run_date DESC, run_number DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data production run")
		return
	}
	defer rows.Close()

	runs := []model.ProductionRun{}
	for rows.Next() {
		var run model.ProductionRun
		if err := scanProductionRun(rows, &run); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data production run")
			return
		}
		runs = append(runs, run)
	}
	writeJSON(w, http.StatusOK, runs)
}

type createProductionRunRequest struct {
	CompanyID   string  `json:"company_id"`
	BranchID    *string `json:"branch_id"`
	WorkOrderID string  `json:"work_order_id"`
	MachineID   string  `json:"machine_id"`
	ShiftID     string  `json:"shift_id"`
	RunDate     string  `json:"run_date"`
	Notes       string  `json:"notes"`
}

// createProductionRun membuka catatan pelaksanaan untuk sebuah work order yang
// SEDANG BERJALAN. Tiga penolakan yang disengaja:
//
//   - work order harus IN_PROGRESS. Mencatat pelaksanaan untuk WO yang masih
//     DRAFT (belum dimulai) atau sudah COMPLETED (angkanya sudah masuk stok)
//     berarti mencatat OEE untuk pekerjaan yang tidak terjadi di jam itu.
//   - mesin harus ACTIVE. Mesin yang sedang MAINTENANCE tidak boleh dibebani
//     waktu shift; kalau tidak, jam perbaikannya masuk ke Availability-nya.
//   - satu mesin hanya boleh punya satu run OPEN (dijaga unique index parsial
//     di database, bukan hanya oleh SELECT di sini).
func (h *Handler) createProductionRun(w http.ResponseWriter, r *http.Request) {
	var req createProductionRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.WorkOrderID == "" || req.MachineID == "" || req.ShiftID == "" || req.RunDate == "" {
		writeError(w, http.StatusBadRequest, "company_id, work_order_id, machine_id, shift_id, dan run_date wajib diisi")
		return
	}
	runDate, err := time.Parse("2006-01-02", req.RunDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "run_date harus format YYYY-MM-DD")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var wo model.WorkOrder
	err = scanWorkOrder(tx.QueryRow(ctx, `SELECT `+workOrderColumns+` FROM work_orders WHERE id = $1 AND company_id = $2`, req.WorkOrderID, req.CompanyID), &wo)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Work order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat work order")
		return
	}
	if wo.Status != "IN_PROGRESS" {
		writeError(w, http.StatusConflict, "Production run hanya bisa dibuat untuk work order berstatus IN_PROGRESS")
		return
	}

	var machine model.Machine
	err = scanMachine(tx.QueryRow(ctx, `SELECT `+machineColumns+` FROM machines WHERE id = $1 AND company_id = $2`, req.MachineID, req.CompanyID), &machine)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Mesin tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat mesin")
		return
	}
	if machine.Status != "ACTIVE" {
		writeError(w, http.StatusConflict, "Mesin ini tidak berstatus ACTIVE")
		return
	}

	var shift model.Shift
	err = scanShift(tx.QueryRow(ctx, `SELECT `+shiftColumns+` FROM shifts WHERE id = $1 AND company_id = $2`, req.ShiftID, req.CompanyID), &shift)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Shift tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat shift")
		return
	}
	if !shift.IsActive {
		writeError(w, http.StatusConflict, "Shift ini sudah nonaktif")
		return
	}

	runNumber, err := nextSequence(ctx, tx, req.CompanyID, "production_runs", "run_number", "PRUN", req.RunDate[:7])
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor production run")
		return
	}

	var run model.ProductionRun
	err = scanProductionRun(tx.QueryRow(ctx, `
		INSERT INTO production_runs (company_id, branch_id, run_number, work_order_id, machine_id, shift_id, run_date, planned_minutes, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+productionRunColumns,
		req.CompanyID, req.BranchID, runNumber, wo.ID, machine.ID, shift.ID, runDate, shift.PlannedMinutes, req.Notes,
	), &run)
	if err != nil {
		if strings.Contains(err.Error(), "idx_production_runs_one_open_per_machine") {
			writeError(w, http.StatusConflict, "Mesin ini masih punya production run yang belum ditutup")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat production run")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan production run")
		return
	}

	h.events.Publish("production.run.created", newAuditEvent("production.run.created", actorFromHeader(r), &run.CompanyID, "create", "production_run", run.ID, run))
	writeJSON(w, http.StatusCreated, productionRunDetail{ProductionRun: run, Machine: &machine, DowntimeLogs: []model.DowntimeLog{}})
}

func (h *Handler) getProductionRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var run model.ProductionRun
	err := scanProductionRun(h.pool.QueryRow(ctx, `SELECT `+productionRunColumns+` FROM production_runs WHERE id = $1`, id), &run)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Production run tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat production run")
		return
	}

	logs, downtime, err := h.fetchDowntime(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat catatan downtime")
		return
	}

	var machine model.Machine
	if err := scanMachine(h.pool.QueryRow(ctx, `SELECT `+machineColumns+` FROM machines WHERE id = $1`, run.MachineID), &machine); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat mesin")
		return
	}

	detail := productionRunDetail{ProductionRun: run, Machine: &machine, DowntimeLogs: logs, DowntimeMinutes: downtime}
	if run.Status == "CLOSED" {
		metrics := computeOEE(run.PlannedMinutes, downtime, valueOrZero(run.QuantityGood), valueOrZero(run.QuantityReject), machine.IdealCycleTimeMinutes)
		detail.OEE = &metrics
	}
	writeJSON(w, http.StatusOK, detail)
}

func valueOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func (h *Handler) fetchDowntime(ctx context.Context, runID string) ([]model.DowntimeLog, int, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, production_run_id, reason_code, minutes, notes, created_at
		FROM downtime_logs WHERE production_run_id = $1 ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := []model.DowntimeLog{}
	total := 0
	for rows.Next() {
		var l model.DowntimeLog
		if err := rows.Scan(&l.ID, &l.ProductionRunID, &l.ReasonCode, &l.Minutes, &l.Notes, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		total += l.Minutes
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

type addDowntimeRequest struct {
	ReasonCode string `json:"reason_code"`
	Minutes    int    `json:"minutes"`
	Notes      string `json:"notes"`
}

// addDowntime hanya untuk run yang masih OPEN, dan total downtime tidak boleh
// melampaui waktu shift: run time negatif membuat Availability negatif dan
// Performance meledak (dibagi angka mendekati nol), jadi ditolak di sini
// alih-alih dibiarkan merusak laporan.
func (h *Handler) addDowntime(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	var req addDowntimeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if !downtimeReasons[req.ReasonCode] {
		writeError(w, http.StatusBadRequest, "reason_code tidak dikenal")
		return
	}
	if req.Minutes <= 0 {
		writeError(w, http.StatusBadRequest, "minutes harus lebih besar dari 0")
		return
	}

	ctx := r.Context()
	var run model.ProductionRun
	err := scanProductionRun(h.pool.QueryRow(ctx, `SELECT `+productionRunColumns+` FROM production_runs WHERE id = $1`, runID), &run)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Production run tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat production run")
		return
	}
	if run.Status != "OPEN" {
		writeError(w, http.StatusConflict, "Production run sudah ditutup")
		return
	}

	_, downtime, err := h.fetchDowntime(ctx, runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat catatan downtime")
		return
	}
	if downtime+req.Minutes > run.PlannedMinutes {
		writeError(w, http.StatusConflict, "Total downtime melebihi waktu shift ("+strconv.Itoa(run.PlannedMinutes)+" menit)")
		return
	}

	var l model.DowntimeLog
	err = h.pool.QueryRow(ctx, `
		INSERT INTO downtime_logs (production_run_id, reason_code, minutes, notes)
		VALUES ($1, $2, $3, $4)
		RETURNING id, production_run_id, reason_code, minutes, notes, created_at`,
		runID, req.ReasonCode, req.Minutes, req.Notes,
	).Scan(&l.ID, &l.ProductionRunID, &l.ReasonCode, &l.Minutes, &l.Notes, &l.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mencatat downtime")
		return
	}

	h.events.Publish("production.downtime.logged", newAuditEvent("production.downtime.logged", actorFromHeader(r), &run.CompanyID, "create", "downtime_log", l.ID, l))
	writeJSON(w, http.StatusCreated, l)
}

func (h *Handler) deleteDowntime(w http.ResponseWriter, r *http.Request) {
	runID, downtimeID := r.PathValue("id"), r.PathValue("downtimeId")
	ctx := r.Context()

	var status string
	err := h.pool.QueryRow(ctx, `SELECT status FROM production_runs WHERE id = $1`, runID).Scan(&status)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Production run tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat production run")
		return
	}
	if status != "OPEN" {
		writeError(w, http.StatusConflict, "Production run sudah ditutup")
		return
	}

	tag, err := h.pool.Exec(ctx, `DELETE FROM downtime_logs WHERE id = $1 AND production_run_id = $2`, downtimeID, runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghapus catatan downtime")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Catatan downtime tidak ditemukan")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type closeProductionRunRequest struct {
	QuantityGood   float64 `json:"quantity_good"`
	QuantityReject float64 `json:"quantity_reject"`
}

// closeProductionRun mengunci angka hasil shift itu, dan sejak itu OEE-nya
// bisa dihitung. Penutupan tidak menyentuh stok sama sekali -- itu tetap
// urusan completeWorkOrder, supaya stok hanya bergerak di satu tempat.
func (h *Handler) closeProductionRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req closeProductionRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.QuantityGood < 0 || req.QuantityReject < 0 {
		writeError(w, http.StatusBadRequest, "quantity_good dan quantity_reject tidak boleh negatif")
		return
	}
	if req.QuantityGood+req.QuantityReject <= 0 {
		writeError(w, http.StatusBadRequest, "Total unit (bagus + reject) harus lebih besar dari 0")
		return
	}

	ctx := r.Context()
	var run model.ProductionRun
	err := scanProductionRun(h.pool.QueryRow(ctx, `
		UPDATE production_runs SET status = 'CLOSED', quantity_good = $1, quantity_reject = $2, updated_at = now()
		WHERE id = $3 AND status = 'OPEN'
		RETURNING `+productionRunColumns, req.QuantityGood, req.QuantityReject, id), &run)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Production run tidak ditemukan atau sudah ditutup")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menutup production run")
		return
	}

	logs, downtime, err := h.fetchDowntime(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat catatan downtime")
		return
	}
	var machine model.Machine
	if err := scanMachine(h.pool.QueryRow(ctx, `SELECT `+machineColumns+` FROM machines WHERE id = $1`, run.MachineID), &machine); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat mesin")
		return
	}
	metrics := computeOEE(run.PlannedMinutes, downtime, valueOrZero(run.QuantityGood), valueOrZero(run.QuantityReject), machine.IdealCycleTimeMinutes)

	h.events.Publish("production.run.closed", newAuditEvent("production.run.closed", actorFromHeader(r), &run.CompanyID, "update", "production_run", run.ID, run))
	writeJSON(w, http.StatusOK, productionRunDetail{
		ProductionRun: run, Machine: &machine, DowntimeLogs: logs, DowntimeMinutes: downtime, OEE: &metrics,
	})
}
