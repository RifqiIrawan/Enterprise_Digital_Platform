package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/production-service/internal/model"
)

// start_time/end_time dibaca sebagai teks "HH:MM", bukan sebagai TIME: jam
// shift tidak punya tanggal maupun zona waktu, dan membungkusnya jadi
// time.Time hanya menghasilkan "0000-01-01T07:00:00Z" yang harus dipotong
// lagi di frontend.
const shiftColumns = `id, company_id, code, name, to_char(start_time, 'HH24:MI'), to_char(end_time, 'HH24:MI'), break_minutes, is_active, created_at, updated_at`

func scanShift(row pgx.Row, s *model.Shift) error {
	if err := row.Scan(&s.ID, &s.CompanyID, &s.Code, &s.Name, &s.StartTime, &s.EndTime, &s.BreakMinutes, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return err
	}
	s.PlannedMinutes = shiftPlannedMinutes(s.StartTime, s.EndTime, s.BreakMinutes)
	return nil
}

// shiftPlannedMinutes mengembalikan menit produktif sebuah shift: durasi jam
// kerja dikurangi istirahat. Shift yang melewati tengah malam (22:00-06:00)
// menghasilkan selisih negatif kalau dihitung lurus, jadi ditambah 24 jam --
// shift malam adalah kasus normal di manufaktur, bukan data yang salah.
func shiftPlannedMinutes(start, end string, breakMinutes int) int {
	startMin, okStart := minutesOfDay(start)
	endMin, okEnd := minutesOfDay(end)
	if !okStart || !okEnd {
		return 0
	}
	duration := endMin - startMin
	if duration <= 0 {
		duration += 24 * 60
	}
	return duration - breakMinutes
}

func minutesOfDay(hhmm string) (int, bool) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

func (h *Handler) listShifts(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT `+shiftColumns+` FROM shifts WHERE company_id = $1 ORDER BY start_time ASC, code ASC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data shift")
		return
	}
	defer rows.Close()

	shifts := []model.Shift{}
	for rows.Next() {
		var s model.Shift
		if err := scanShift(rows, &s); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data shift")
			return
		}
		shifts = append(shifts, s)
	}
	writeJSON(w, http.StatusOK, shifts)
}

type shiftRequest struct {
	CompanyID    string `json:"company_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	BreakMinutes int    `json:"break_minutes"`
	IsActive     bool   `json:"is_active"`
}

// validateShiftTimes memastikan jam yang masuk benar-benar bisa dipakai
// sebagai penyebut Availability: formatnya jam, dan istirahatnya tidak
// menghabiskan (atau melebihi) shift itu sendiri.
func validateShiftTimes(start, end string, breakMinutes int) (int, string) {
	if _, ok := minutesOfDay(start); !ok {
		return 0, "start_time harus format HH:MM"
	}
	if _, ok := minutesOfDay(end); !ok {
		return 0, "end_time harus format HH:MM"
	}
	if start == end {
		return 0, "start_time dan end_time tidak boleh sama"
	}
	if breakMinutes < 0 {
		return 0, "break_minutes tidak boleh negatif"
	}
	planned := shiftPlannedMinutes(start, end, breakMinutes)
	if planned <= 0 {
		return 0, "break_minutes menghabiskan seluruh durasi shift"
	}
	return planned, ""
}

func (h *Handler) createShift(w http.ResponseWriter, r *http.Request) {
	var req shiftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.Code == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, code, dan name wajib diisi")
		return
	}
	if _, msg := validateShiftTimes(req.StartTime, req.EndTime, req.BreakMinutes); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var s model.Shift
	err := scanShift(h.pool.QueryRow(r.Context(), `
		INSERT INTO shifts (company_id, code, name, start_time, end_time, break_minutes)
		VALUES ($1, $2, $3, $4::time, $5::time, $6)
		RETURNING `+shiftColumns,
		req.CompanyID, req.Code, req.Name, req.StartTime, req.EndTime, req.BreakMinutes,
	), &s)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode shift sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat shift")
		return
	}

	h.events.Publish("production.shift.created", newAuditEvent("production.shift.created", actorFromHeader(r), &s.CompanyID, "create", "shift", s.ID, s))
	writeJSON(w, http.StatusCreated, s)
}

// updateShift boleh mengubah jam kapan saja: production run yang sudah ada
// membawa planned_minutes-nya sendiri (snapshot saat run dibuat), jadi
// mengubah shift tidak menulis ulang OEE periode yang sudah lewat.
func (h *Handler) updateShift(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req shiftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}
	if _, msg := validateShiftTimes(req.StartTime, req.EndTime, req.BreakMinutes); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var s model.Shift
	err := scanShift(h.pool.QueryRow(r.Context(), `
		UPDATE shifts SET name = $1, start_time = $2::time, end_time = $3::time, break_minutes = $4, is_active = $5, updated_at = now()
		WHERE id = $6
		RETURNING `+shiftColumns,
		req.Name, req.StartTime, req.EndTime, req.BreakMinutes, req.IsActive, id,
	), &s)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Shift tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui shift")
		return
	}

	h.events.Publish("production.shift.updated", newAuditEvent("production.shift.updated", actorFromHeader(r), &s.CompanyID, "update", "shift", s.ID, s))
	writeJSON(w, http.StatusOK, s)
}
