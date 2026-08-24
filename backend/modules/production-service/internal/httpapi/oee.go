package httpapi

import (
	"math"
	"net/http"
	"strconv"
)

// Ringkasan OEE per mesin untuk sebuah periode, plus Pareto penyebab downtime.
// Hanya run berstatus CLOSED yang ikut dihitung: run yang masih berjalan belum
// punya jumlah unit, dan memasukkannya hanya akan menyeret Availability ke
// bawah untuk shift yang bahkan belum selesai.

type machineOEE struct {
	MachineID   string `json:"machine_id"`
	MachineCode string `json:"machine_code"`
	MachineName string `json:"machine_name"`
	RunCount    int    `json:"run_count"`
	oeeMetrics
}

type downtimeReasonShare struct {
	ReasonCode string  `json:"reason_code"`
	Minutes    int     `json:"minutes"`
	Share      float64 `json:"share"`
}

type oeeResponse struct {
	Overall          oeeMetrics            `json:"overall"`
	Machines         []machineOEE          `json:"machines"`
	DowntimeByReason []downtimeReasonShare `json:"downtime_by_reason"`
}

// aggregateOEE adalah inti perhitungan: idealMinutes adalah total menit yang
// SEHARUSNYA dibutuhkan untuk unit yang dihasilkan. Dipisah dari computeOEE
// karena agregat lintas mesin tidak boleh memakai satu ideal cycle time --
// tiap mesin punya kecepatan rancangannya sendiri, jadi menit idealnya
// dijumlahkan per mesin lebih dulu, baru dibandingkan dengan total run time.
func aggregateOEE(plannedMinutes, downtimeMinutes int, good, reject, idealMinutes float64) oeeMetrics {
	m := oeeMetrics{
		PlannedMinutes:  plannedMinutes,
		DowntimeMinutes: downtimeMinutes,
		QuantityGood:    good,
		QuantityReject:  reject,
	}
	m.RunTimeMinutes = plannedMinutes - downtimeMinutes
	if m.RunTimeMinutes < 0 {
		m.RunTimeMinutes = 0
	}
	if plannedMinutes > 0 {
		m.Availability = float64(m.RunTimeMinutes) / float64(plannedMinutes)
	}
	total := good + reject
	if m.RunTimeMinutes > 0 && idealMinutes > 0 {
		m.Performance = idealMinutes / float64(m.RunTimeMinutes)
		if m.Performance > 1 {
			m.Performance = 1
			m.PerformanceCapped = true
		}
	}
	if total > 0 {
		m.Quality = good / total
	}
	m.OEE = m.Availability * m.Performance * m.Quality
	return round4(m)
}

func (h *Handler) oeeSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	companyID := q.Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	// Filter dibangun sekali dan dipakai dua kali (ringkasan mesin & Pareto
	// downtime) supaya keduanya tidak bisa diam-diam menghitung periode yang
	// berbeda.
	where := ` WHERE pr.company_id = $1 AND pr.status = 'CLOSED'`
	args := []any{companyID}
	addFilter := func(value, clause string) {
		if value == "" {
			return
		}
		args = append(args, value)
		where += ` AND ` + clause + ` $` + strconv.Itoa(len(args))
	}
	if branchID := q.Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		where += ` AND (pr.branch_id = $` + strconv.Itoa(len(args)) + ` OR pr.branch_id IS NULL)`
	}
	addFilter(q.Get("machine_id"), `pr.machine_id =`)
	addFilter(q.Get("from"), `pr.run_date >=`)
	addFilter(q.Get("to"), `pr.run_date <=`)

	// LATERAL, bukan JOIN biasa ke downtime_logs: sebuah run dengan 3 catatan
	// downtime akan menggandakan planned_minutes-nya 3 kali kalau di-join
	// langsung, dan Availability-nya jadi tidak masuk akal.
	rows, err := h.pool.Query(r.Context(), `
		SELECT m.id, m.code, m.name, m.ideal_cycle_time_minutes,
		       COUNT(pr.id),
		       COALESCE(SUM(pr.planned_minutes), 0),
		       COALESCE(SUM(pr.quantity_good), 0),
		       COALESCE(SUM(pr.quantity_reject), 0),
		       COALESCE(SUM(dt.minutes), 0)
		FROM production_runs pr
		JOIN machines m ON m.id = pr.machine_id
		LEFT JOIN LATERAL (
		    SELECT COALESCE(SUM(minutes), 0) AS minutes FROM downtime_logs WHERE production_run_id = pr.id
		) dt ON TRUE`+where+`
		GROUP BY m.id, m.code, m.name, m.ideal_cycle_time_minutes
		ORDER BY m.code ASC`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghitung OEE")
		return
	}
	defer rows.Close()

	resp := oeeResponse{Machines: []machineOEE{}, DowntimeByReason: []downtimeReasonShare{}}
	var totalPlanned, totalDowntime int
	var totalGood, totalReject, totalIdealMinutes float64
	for rows.Next() {
		var (
			mo        machineOEE
			idealTime float64
			planned   int
			downtime  int
			good      float64
			reject    float64
		)
		if err := rows.Scan(&mo.MachineID, &mo.MachineCode, &mo.MachineName, &idealTime, &mo.RunCount,
			&planned, &good, &reject, &downtime); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca hasil perhitungan OEE")
			return
		}
		idealMinutes := idealTime * (good + reject)
		mo.oeeMetrics = aggregateOEE(planned, downtime, good, reject, idealMinutes)
		resp.Machines = append(resp.Machines, mo)

		totalPlanned += planned
		totalDowntime += downtime
		totalGood += good
		totalReject += reject
		totalIdealMinutes += idealMinutes
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca hasil perhitungan OEE")
		return
	}
	resp.Overall = aggregateOEE(totalPlanned, totalDowntime, totalGood, totalReject, totalIdealMinutes)

	reasonRows, err := h.pool.Query(r.Context(), `
		SELECT d.reason_code, SUM(d.minutes)
		FROM downtime_logs d
		JOIN production_runs pr ON pr.id = d.production_run_id`+where+`
		GROUP BY d.reason_code
		ORDER BY SUM(d.minutes) DESC`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat rincian downtime")
		return
	}
	defer reasonRows.Close()

	for reasonRows.Next() {
		var share downtimeReasonShare
		if err := reasonRows.Scan(&share.ReasonCode, &share.Minutes); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca rincian downtime")
			return
		}
		if totalDowntime > 0 {
			share.Share = math.Round(float64(share.Minutes)/float64(totalDowntime)*10000) / 10000
		}
		resp.DowntimeByReason = append(resp.DowntimeByReason, share)
	}
	if err := reasonRows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca rincian downtime")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
