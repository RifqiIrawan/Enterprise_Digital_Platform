package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/enterprise-digital-platform/hr-service/internal/model"
)

// Kuota cuti tahunan. Sebelum ini `total_days` di pengajuan cuti hanya angka
// tampilan: tidak ada jatah yang diperiksa, jadi karyawan bisa mengambil cuti
// tahunan tanpa batas.
//
// Dua keputusan yang menentukan bentuk seluruh file ini:
//
//  1. Hari terpakai TIDAK disimpan. Selalu dihitung ulang dari leave_requests
//     berstatus APPROVED (lihat usedAnnualLeaveDays). Kalau disimpan, angka itu
//     harus ikut dikoreksi tiap kali cuti dibatalkan/ditolak/diubah, dan cepat
//     atau lambat akan lepas sinkron dengan data cutinya sendiri.
//  2. Kuota hanya berlaku untuk cuti ANNUAL. Sakit, melahirkan, dan cuti tanpa
//     gaji punya aturannya sendiri di UU dan tidak memotong jatah tahunan.

const defaultAnnualQuotaDays = 12 // batas minimum UU Ketenagakerjaan

type leaveQuotaView struct {
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	Year         int    `json:"year"`
	TotalDays    int    `json:"total_days"`
	CarriedOver  int    `json:"carried_over"`
	// UsedDays dihitung dari cuti ANNUAL yang APPROVED pada tahun itu.
	UsedDays int `json:"used_days"`
	// RemainingDays boleh negatif kalau kuota diturunkan setelah cuti terlanjur
	// disetujui -- ditampilkan apa adanya, bukan dipangkas jadi 0, supaya
	// selisihnya kelihatan.
	RemainingDays int    `json:"remaining_days"`
	Note          string `json:"note"`
	// HasQuotaRow membedakan "memang belum pernah diatur" (memakai default)
	// dari "diatur persis sebesar default".
	HasQuotaRow bool `json:"has_quota_row"`
}

func (h *Handler) listLeaveQuotas(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	year, msg := parseYearParam(r.URL.Query().Get("year"))
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	employeeID := r.URL.Query().Get("employee_id")

	// Semua karyawan aktif selalu muncul, termasuk yang belum punya baris kuota
	// (LEFT JOIN + COALESCE ke default) -- kalau tidak, HR harus menebak siapa
	// yang belum diatur.
	rows, err := h.pool.Query(r.Context(), `
		SELECT e.id, TRIM(e.first_name || ' ' || COALESCE(e.last_name, '')),
		       COALESCE(q.total_days, $3), COALESCE(q.carried_over, 0),
		       COALESCE(q.note, ''), (q.id IS NOT NULL),
		       COALESCE((
		         SELECT COUNT(*)
		         FROM leave_requests lr
		         CROSS JOIN LATERAL generate_series(lr.start_date::timestamp, lr.end_date::timestamp, INTERVAL '1 day') AS d
		         WHERE lr.employee_id = e.id
		           AND lr.status = 'APPROVED'
		           AND lr.leave_type = 'ANNUAL'
		           AND EXTRACT(YEAR FROM d) = $2
		           AND EXTRACT(ISODOW FROM d) < 6
		           AND NOT EXISTS (
		             SELECT 1 FROM holidays hd
		             WHERE hd.company_id = lr.company_id AND hd.holiday_date = d::date
		           )
		       ), 0)
		FROM employees e
		LEFT JOIN leave_quotas q ON q.employee_id = e.id AND q.year = $2
		WHERE e.company_id = $1 AND e.status = 'ACTIVE'
		  AND ($4 = '' OR e.id = $4::uuid)
		ORDER BY e.first_name ASC, e.last_name ASC`, companyID, year, defaultAnnualQuotaDays, employeeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat kuota cuti")
		return
	}
	defer rows.Close()

	views := []leaveQuotaView{}
	for rows.Next() {
		v := leaveQuotaView{Year: year}
		if err := rows.Scan(&v.EmployeeID, &v.EmployeeName, &v.TotalDays, &v.CarriedOver, &v.Note, &v.HasQuotaRow, &v.UsedDays); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca kuota cuti")
			return
		}
		v.RemainingDays = v.TotalDays + v.CarriedOver - v.UsedDays
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, views)
}

type putLeaveQuotaRequest struct {
	EmployeeID  string `json:"employee_id"`
	Year        int    `json:"year"`
	TotalDays   *int   `json:"total_days"`
	CarriedOver *int   `json:"carried_over"`
	Note        string `json:"note"`
}

// putLeaveQuota membuat atau memperbarui kuota satu karyawan untuk satu tahun.
// PUT (bukan POST) karena pemanggil tidak perlu tahu barisnya sudah ada atau
// belum -- UI hanya mengirim jatah yang diinginkan.
func (h *Handler) putLeaveQuota(w http.ResponseWriter, r *http.Request) {
	var req putLeaveQuotaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.EmployeeID == "" || req.Year == 0 {
		writeError(w, http.StatusBadRequest, "employee_id dan year wajib diisi")
		return
	}
	if req.Year < 2000 || req.Year > 2100 {
		writeError(w, http.StatusBadRequest, "year di luar rentang yang masuk akal")
		return
	}
	totalDays := defaultAnnualQuotaDays
	if req.TotalDays != nil {
		totalDays = *req.TotalDays
	}
	carriedOver := 0
	if req.CarriedOver != nil {
		carriedOver = *req.CarriedOver
	}
	if totalDays < 0 || carriedOver < 0 {
		writeError(w, http.StatusBadRequest, "Jatah cuti tidak boleh negatif")
		return
	}

	ctx := r.Context()
	var companyID string
	if err := h.pool.QueryRow(ctx,
		`SELECT company_id FROM employees WHERE id = $1`, req.EmployeeID).Scan(&companyID); err != nil {
		writeError(w, http.StatusNotFound, "Karyawan tidak ditemukan")
		return
	}

	if _, err := h.pool.Exec(ctx, `
		INSERT INTO leave_quotas (employee_id, year, total_days, carried_over, note)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (employee_id, year) DO UPDATE
		SET total_days = EXCLUDED.total_days,
		    carried_over = EXCLUDED.carried_over,
		    note = EXCLUDED.note,
		    updated_at = now()`,
		req.EmployeeID, req.Year, totalDays, carriedOver, nullIfEmpty(req.Note)); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan kuota cuti")
		return
	}

	view, err := h.leaveQuotaFor(ctx, req.EmployeeID, req.Year)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca kuota cuti")
		return
	}

	h.events.Publish("hr.leave_quota.updated", newAuditEvent("hr.leave_quota.updated", actorFromHeader(r), &companyID, "update", "leave_quota", req.EmployeeID, view))
	writeJSON(w, http.StatusOK, view)
}

// leaveQuotaFor membaca kuota efektif satu karyawan pada satu tahun, memakai
// default kalau belum pernah diatur.
func (h *Handler) leaveQuotaFor(ctx context.Context, employeeID string, year int) (leaveQuotaView, error) {
	v := leaveQuotaView{EmployeeID: employeeID, Year: year}
	err := h.pool.QueryRow(ctx, `
		SELECT TRIM(e.first_name || ' ' || COALESCE(e.last_name, '')),
		       COALESCE(q.total_days, $3), COALESCE(q.carried_over, 0),
		       COALESCE(q.note, ''), (q.id IS NOT NULL)
		FROM employees e
		LEFT JOIN leave_quotas q ON q.employee_id = e.id AND q.year = $2
		WHERE e.id = $1`, employeeID, year, defaultAnnualQuotaDays).
		Scan(&v.EmployeeName, &v.TotalDays, &v.CarriedOver, &v.Note, &v.HasQuotaRow)
	if err != nil {
		return v, err
	}
	used, err := h.usedAnnualLeaveDays(ctx, employeeID, year, "")
	if err != nil {
		return v, err
	}
	v.UsedDays = used
	v.RemainingDays = v.TotalDays + v.CarriedOver - v.UsedDays
	return v, nil
}

// usedAnnualLeaveDays menghitung hari kerja cuti ANNUAL yang sudah APPROVED
// pada satu tahun. excludeID dipakai saat memeriksa sebuah pengajuan yang
// sedang disetujui, supaya pengajuan itu sendiri tidak ikut terhitung dua kali
// (persetujuan ulang setelah pembatalan, misalnya).
//
// Rentang cuti dipecah per hari lewat generate_series -- bukan dibaca dari
// kolom total_days -- supaya cuti yang menyeberang tahun hanya membebani jatah
// tahun yang benar. Akhir pekan dan hari libur dibuang dengan aturan yang sama
// seperti perhitungan hari kerja di tempat lain.
func (h *Handler) usedAnnualLeaveDays(ctx context.Context, employeeID string, year int, excludeID string) (int, error) {
	var used int
	err := h.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM leave_requests lr
		CROSS JOIN LATERAL generate_series(lr.start_date::timestamp, lr.end_date::timestamp, INTERVAL '1 day') AS d
		WHERE lr.employee_id = $1
		  AND lr.status = 'APPROVED'
		  AND lr.leave_type = 'ANNUAL'
		  AND EXTRACT(YEAR FROM d) = $2
		  AND EXTRACT(ISODOW FROM d) < 6
		  AND ($3 = '' OR lr.id <> $3::uuid)
		  AND NOT EXISTS (
		    SELECT 1 FROM holidays hd
		    WHERE hd.company_id = lr.company_id AND hd.holiday_date = d::date
		  )`, employeeID, year, excludeID).Scan(&used)
	return used, err
}

// annualQuotaCheck memeriksa apakah sebuah pengajuan cuti ANNUAL masih muat di
// jatah tahunannya. Dipanggil saat PERSETUJUAN, bukan saat pengajuan dibuat:
// yang benar-benar memakan jatah adalah cuti yang disetujui, dan menahan
// pengajuan sejak awal hanya memindahkan percakapannya ke luar sistem.
//
// Cuti yang menyeberang tahun diperiksa per tahun: 3 hari di Desember dan 2
// hari di Januari membebani dua jatah yang berbeda.
func (h *Handler) annualQuotaCheck(ctx context.Context, l model.LeaveRequest) (string, error) {
	if l.LeaveType != "ANNUAL" {
		return "", nil
	}
	holidays, err := h.holidaySet(ctx, l.CompanyID, l.StartDate, l.EndDate)
	if err != nil {
		return "", err
	}

	perYear := map[int]int{}
	for d := l.StartDate; !d.After(l.EndDate); d = d.AddDate(0, 0, 1) {
		if isWeekend(d) || holidays[d.Format("2006-01-02")] {
			continue
		}
		perYear[d.Year()]++
	}

	for year, days := range perYear {
		quota, err := h.leaveQuotaFor(ctx, l.EmployeeID, year)
		if err != nil {
			return "", err
		}
		used, err := h.usedAnnualLeaveDays(ctx, l.EmployeeID, year, l.ID)
		if err != nil {
			return "", err
		}
		available := quota.TotalDays + quota.CarriedOver - used
		if days > available {
			return "Jatah cuti tahunan " + strconv.Itoa(year) + " tidak cukup: butuh " +
				strconv.Itoa(days) + " hari, sisa " + strconv.Itoa(available) + " hari", nil
		}
	}
	return "", nil
}

func parseYearParam(raw string) (int, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().Year(), ""
	}
	year, err := strconv.Atoi(raw)
	if err != nil {
		return 0, "year harus berupa angka"
	}
	if year < 2000 || year > 2100 {
		return 0, "year di luar rentang yang masuk akal"
	}
	return year, ""
}
