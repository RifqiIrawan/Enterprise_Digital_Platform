package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/hr-service/internal/model"
)

// Kalender hari libur: satu-satunya sumber "tanggal ini hari kerja atau bukan"
// di hr-service. Sebelum ada file ini, seluruh perhitungan hari kerja
// (pro-rata payroll, total hari cuti, penentuan tarif lembur) memakai aturan
// Senin-Jumat polos, sehingga tanggal merah tetap dihitung sebagai hari kerja.
//
// Aturannya sengaja satu kalimat: hari kerja = bukan Sabtu/Minggu DAN tidak ada
// di tabel holidays milik company itu.

type holidayPayload struct {
	CompanyID   string `json:"company_id"`
	HolidayDate string `json:"holiday_date"`
	Name        string `json:"name"`
	IsNational  *bool  `json:"is_national"`
}

const holidayColumns = `id, company_id, holiday_date, name, is_national, created_by, created_at, updated_at`

func scanHoliday(row pgx.Row, h *model.Holiday) error {
	return row.Scan(&h.ID, &h.CompanyID, &h.HolidayDate, &h.Name, &h.IsNational, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt)
}

// listHolidays mengembalikan kalender satu company, opsional disaring per tahun
// (year=2026). Diurutkan menaik supaya bisa langsung dipakai UI kalender.
func (h *Handler) listHolidays(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	year := strings.TrimSpace(r.URL.Query().Get("year"))
	if year != "" {
		if _, err := strconv.Atoi(year); err != nil {
			writeError(w, http.StatusBadRequest, "year harus berupa angka")
			return
		}
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT `+holidayColumns+`
		FROM holidays
		WHERE company_id = $1 AND ($2 = '' OR EXTRACT(YEAR FROM holiday_date)::text = $2)
		ORDER BY holiday_date ASC`, companyID, year)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat kalender hari libur")
		return
	}
	defer rows.Close()

	holidays := []model.Holiday{}
	for rows.Next() {
		var item model.Holiday
		if err := scanHoliday(rows, &item); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca kalender hari libur")
			return
		}
		holidays = append(holidays, item)
	}
	writeJSON(w, http.StatusOK, holidays)
}

func (h *Handler) createHoliday(w http.ResponseWriter, r *http.Request) {
	var req holidayPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.HolidayDate == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, holiday_date, dan name wajib diisi")
		return
	}
	date, err := time.Parse("2006-01-02", req.HolidayDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "holiday_date harus berformat YYYY-MM-DD")
		return
	}
	isNational := true
	if req.IsNational != nil {
		isNational = *req.IsNational
	}

	var item model.Holiday
	err = scanHoliday(h.pool.QueryRow(r.Context(), `
		INSERT INTO holidays (company_id, holiday_date, name, is_national, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+holidayColumns,
		req.CompanyID, date, req.Name, isNational, actorFromHeader(r)), &item)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Tanggal itu sudah ada di kalender company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan hari libur")
		return
	}

	h.events.Publish("hr.holiday.created", newAuditEvent("hr.holiday.created", actorFromHeader(r), &item.CompanyID, "create", "holiday", item.ID, item))
	writeJSON(w, http.StatusCreated, item)
}

// deleteHoliday: hari libur yang salah input dihapus, bukan dinonaktifkan.
// Perhitungan hari kerja SELALU dibaca ulang dari tabel ini, jadi menghapus
// baris otomatis mengembalikan tanggal itu menjadi hari kerja -- termasuk untuk
// payroll periode yang belum diproses. Payroll yang sudah POSTED tidak ikut
// berubah karena angkanya sudah tersimpan di payroll_details.
func (h *Handler) deleteHoliday(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var companyID string
	err := h.pool.QueryRow(r.Context(),
		`DELETE FROM holidays WHERE id = $1 RETURNING company_id`, id).Scan(&companyID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Hari libur tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghapus hari libur")
		return
	}

	h.events.Publish("hr.holiday.deleted", newAuditEvent("hr.holiday.deleted", actorFromHeader(r), &companyID, "delete", "holiday", id, nil))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// holidaySet mengambil tanggal libur satu company dalam rentang tertentu
// sebagai himpunan "YYYY-MM-DD", supaya pemanggil bisa memeriksa banyak tanggal
// tanpa bolak-balik ke database.
func (h *Handler) holidaySet(ctx context.Context, companyID string, start, end time.Time) (map[string]bool, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT holiday_date FROM holidays
		WHERE company_id = $1 AND holiday_date BETWEEN $2 AND $3`, companyID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	set := map[string]bool{}
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		set[d.Format("2006-01-02")] = true
	}
	return set, rows.Err()
}

// workingDaysBetween menghitung hari kerja dalam rentang inklusif: bukan akhir
// pekan DAN bukan hari libur di kalender company tersebut.
func (h *Handler) workingDaysBetween(ctx context.Context, companyID string, start, end time.Time) (int, error) {
	holidays, err := h.holidaySet(ctx, companyID, start, end)
	if err != nil {
		return 0, err
	}
	count := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if isWeekend(d) || holidays[d.Format("2006-01-02")] {
			continue
		}
		count++
	}
	return count, nil
}

// workingDaysInMonth adalah penyebut pro-rata payroll: jumlah hari kerja pada
// bulan yang memuat monthStart.
func (h *Handler) workingDaysInMonth(ctx context.Context, companyID string, monthStart time.Time) (int, error) {
	monthEnd := monthStart.AddDate(0, 1, -1)
	return h.workingDaysBetween(ctx, companyID, monthStart, monthEnd)
}

// isNonWorkingDay dipakai lembur: akhir pekan ATAU tanggal merah sama-sama
// berarti tarif lembur hari libur (Kepmenaker 102/2004), tanpa bergantung pada
// ketelitian orang mencentang checkbox.
func (h *Handler) isNonWorkingDay(ctx context.Context, companyID string, date time.Time) (bool, error) {
	if isWeekend(date) {
		return true, nil
	}
	var exists bool
	err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM holidays WHERE company_id = $1 AND holiday_date = $2)`,
		companyID, date).Scan(&exists)
	return exists, err
}

func isWeekend(d time.Time) bool {
	return d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
}
