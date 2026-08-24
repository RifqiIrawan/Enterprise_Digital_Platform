package httpapi

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Predictive maintenance (Fase 10). Sama seperti forecasting dan anomaly
// detection di service ini, ini HEURISTIK YANG DIJELASKAN, bukan model yang
// dilatih: setiap angka risiko datang dari faktor-faktor yang ikut dikirim
// beserta kontribusinya masing-masing, sehingga orang yang membacanya bisa
// menyanggahnya. Skor tanpa alasan hanya akan diabaikan setelah tebakan
// pertamanya meleset.
//
// Dua sumber sinyal, keduanya sudah ada di platform ini:
//
//   - MESIN (production-service): OEE per mesin -- availability yang rendah
//     berarti mesin sering berhenti, dan mesin yang sering berhenti biasanya
//     memang sedang menuju kerusakan.
//   - ASET (asset-service): jadwal maintenance yang lewat, aset yang belum
//     pernah dirawat sama sekali, dan aset yang nilai bukunya sudah mendekati
//     nilai residu (tanda umur manfaatnya hampir habis -- data yang baru ada
//     sejak penyusutan dibangun di Fase 5).
//
// IoT SENGAJA tidak dipakai: device di iot-service hanya menyimpan
// warehouse_id, tidak ada kolom yang menghubungkannya ke aset atau mesin
// tertentu. Mencocokkan lewat kemiripan nama adalah tebakan yang akan salah
// tanpa ada yang menyadarinya; kalau kaitan itu nanti benar-benar ada
// (device.asset_id), alertnya tinggal ditambahkan sebagai faktor baru di sini.

type riskFactor struct {
	Code         string  `json:"code"`
	Label        string  `json:"label"`
	Contribution float64 `json:"contribution"`
	Detail       string  `json:"detail"`
}

type maintenanceRisk struct {
	SubjectType string       `json:"subject_type"` // "machine" | "asset"
	SubjectID   string       `json:"subject_id"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	RiskScore   float64      `json:"risk_score"`
	RiskLevel   string       `json:"risk_level"`
	Factors     []riskFactor `json:"factors"`
	Action      string       `json:"recommended_action"`
}

type predictiveResponse struct {
	CompanyID   string            `json:"company_id"`
	GeneratedAt time.Time         `json:"generated_at"`
	Items       []maintenanceRisk `json:"items"`
	Errors      []sourceError     `json:"errors"`
}

// Ambang tingkat risiko. Angkanya dipilih supaya satu faktor besar saja
// (mis. availability 60%) belum cukup untuk HIGH -- perlu dua hal yang
// bersamaan buruk. Mesin yang naik ke HIGH karena satu angka tunggal akan
// membuat daftar ini penuh dan tidak lagi dibaca.
const (
	riskHighThreshold   = 60
	riskMediumThreshold = 30
)

func riskLevel(score float64) string {
	switch {
	case score >= riskHighThreshold:
		return "HIGH"
	case score >= riskMediumThreshold:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// machineOEEView adalah bagian ringkasan OEE production-service yang dipakai di
// sini (lihat backend/modules/production-service/internal/httpapi/oee.go).
type machineOEEView struct {
	MachineID    string  `json:"machine_id"`
	MachineCode  string  `json:"machine_code"`
	MachineName  string  `json:"machine_name"`
	RunCount     int     `json:"run_count"`
	Availability float64 `json:"availability"`
	Quality      float64 `json:"quality"`
	OEE          float64 `json:"oee"`
	Downtime     int     `json:"downtime_minutes"`
	Planned      int     `json:"planned_minutes"`
}

type machineView struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type assetView struct {
	ID                      string  `json:"id"`
	AssetCode               string  `json:"asset_code"`
	Name                    string  `json:"name"`
	Status                  string  `json:"status"`
	AcquisitionCost         float64 `json:"acquisition_cost"`
	SalvageValue            float64 `json:"salvage_value"`
	UsefulLifeMonths        *int    `json:"useful_life_months"`
	AccumulatedDepreciation float64 `json:"accumulated_depreciation"`
	BookValue               float64 `json:"book_value"`
	Category                string  `json:"category"`
}

type maintenanceScheduleView struct {
	ID            string `json:"id"`
	AssetID       string `json:"asset_id"`
	ScheduledDate string `json:"scheduled_date"`
	Status        string `json:"status"`
}

func (h *Handler) predictiveMaintenanceScan(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	resp := predictiveResponse{
		CompanyID:   companyID,
		GeneratedAt: time.Now(),
		Items:       []maintenanceRisk{},
		Errors:      []sourceError{},
	}
	var mu sync.Mutex
	addErr := func(source string, err error) {
		mu.Lock()
		defer mu.Unlock()
		resp.Errors = append(resp.Errors, sourceError{Source: source, Message: err.Error()})
	}

	var oee struct {
		Machines []machineOEEView `json:"machines"`
	}
	var (
		machines  []machineView
		assets    []assetView
		schedules []maintenanceScheduleView
	)

	var wg sync.WaitGroup
	wg.Go(func() {
		if err := h.getJSON(h.cfg.ProductionServiceURL, "/oee", companyID, &oee); err != nil {
			addErr("production-service", err)
		}
	})
	wg.Go(func() {
		if err := h.getJSON(h.cfg.ProductionServiceURL, "/machines", companyID, &machines); err != nil {
			addErr("production-service", err)
		}
	})
	wg.Go(func() {
		if err := h.getJSON(h.cfg.AssetServiceURL, "/assets", companyID, &assets); err != nil {
			addErr("asset-service", err)
		}
	})
	wg.Go(func() {
		if err := h.getJSON(h.cfg.AssetServiceURL, "/maintenance-schedules", companyID, &schedules); err != nil {
			addErr("asset-service", err)
		}
	})
	wg.Wait()

	statusByMachine := map[string]string{}
	for _, m := range machines {
		statusByMachine[m.ID] = m.Status
	}
	for _, m := range oee.Machines {
		if item, ok := scoreMachine(m, statusByMachine[m.MachineID]); ok {
			resp.Items = append(resp.Items, item)
		}
	}
	resp.Items = append(resp.Items, scoreAssets(assets, schedules, time.Now())...)

	// Urut dari yang paling perlu dilihat; nama dipakai sebagai pemecah seri
	// supaya urutannya stabil antar panggilan (dua subjek berskor sama tidak
	// boleh bertukar tempat setiap kali halaman dimuat ulang).
	sort.Slice(resp.Items, func(i, j int) bool {
		if resp.Items[i].RiskScore != resp.Items[j].RiskScore {
			return resp.Items[i].RiskScore > resp.Items[j].RiskScore
		}
		return resp.Items[i].Code < resp.Items[j].Code
	})
	writeJSON(w, http.StatusOK, resp)
}

// scoreMachine menilai satu mesin dari ringkasan OEE-nya. Mesin tanpa satu pun
// run tertutup TIDAK dinilai: tidak ada bukti apa pun tentangnya, dan
// memberinya skor 0 sama menyesatkannya dengan memberinya skor tinggi.
func scoreMachine(m machineOEEView, status string) (maintenanceRisk, bool) {
	if m.RunCount == 0 {
		return maintenanceRisk{}, false
	}

	item := maintenanceRisk{
		SubjectType: "machine",
		SubjectID:   m.MachineID,
		Code:        m.MachineCode,
		Name:        m.MachineName,
		Factors:     []riskFactor{},
	}

	// Availability rendah = mesin sering berhenti. Ini sinyal terkuat yang
	// benar-benar dimiliki platform ini tentang kesehatan mesin.
	if m.Availability < 0.85 {
		points := math.Min((0.85-m.Availability)*200, 45)
		item.Factors = append(item.Factors, riskFactor{
			Code:         "LOW_AVAILABILITY",
			Label:        "Availability di bawah 85%",
			Contribution: round1(points),
			Detail:       formatPercent(m.Availability) + " availability, " + itoa(m.Downtime) + " menit berhenti dari " + itoa(m.Planned) + " menit terjadwal",
		})
	}

	// Reject yang menetap sering berarti perkakas/setelan yang sudah aus --
	// bobotnya lebih kecil karena mutu juga bisa turun oleh sebab lain
	// (bahan baku, operator), bukan oleh mesinnya.
	if m.Quality < 0.95 {
		points := math.Min((0.95-m.Quality)*200, 20)
		item.Factors = append(item.Factors, riskFactor{
			Code:         "QUALITY_DRAG",
			Label:        "Tingkat reject tinggi",
			Contribution: round1(points),
			Detail:       formatPercent(m.Quality) + " unit lolos",
		})
	}

	if status == "MAINTENANCE" {
		item.Factors = append(item.Factors, riskFactor{
			Code:         "IN_MAINTENANCE",
			Label:        "Mesin sedang berstatus MAINTENANCE",
			Contribution: 15,
			Detail:       "Sudah ditarik dari produksi",
		})
	}

	if len(item.Factors) == 0 {
		return maintenanceRisk{}, false
	}
	item.RiskScore = round1(totalContribution(item.Factors))
	item.RiskLevel = riskLevel(item.RiskScore)
	item.Action = machineAction(item)
	return item, true
}

func machineAction(item maintenanceRisk) string {
	switch topFactor(item.Factors) {
	case "LOW_AVAILABILITY":
		return "Periksa penyebab downtime terbesar mesin ini di halaman Eksekusi Produksi sebelum menjadwalkan perbaikan"
	case "QUALITY_DRAG":
		return "Periksa keausan perkakas dan setelan mesin; reject yang menetap jarang membaik sendiri"
	default:
		return "Pastikan perbaikan yang sedang berjalan selesai sebelum mesin dijadwalkan produksi lagi"
	}
}

// scoreAssets menilai aset dari jadwal maintenance-nya dan dari sisa umur
// manfaatnya. `now` di-inject supaya perhitungan "berapa hari terlambat" bisa
// diuji tanpa bergantung pada tanggal hari ini.
func scoreAssets(assets []assetView, schedules []maintenanceScheduleView, now time.Time) []maintenanceRisk {
	// Tanggal jadwal datang sebagai tanggal saja (tanpa jam, tanpa zona), jadi
	// pembandingnya harus tanggal juga. Truncate(24h) pada timestamp SALAH di
	// sini: dia memotong di tengah malam UTC, sehingga di zona waktu Indonesia
	// (+07) hasilnya bergeser satu hari selama tujuh jam pertama tiap harinya.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	overdueDays := map[string]int{}
	completedCount := map[string]int{}
	for _, s := range schedules {
		switch s.Status {
		case "SCHEDULED":
			d, err := time.Parse("2006-01-02", firstTen(s.ScheduledDate))
			if err != nil {
				continue
			}
			days := int(today.Sub(d).Hours() / 24)
			if days > overdueDays[s.AssetID] {
				overdueDays[s.AssetID] = days
			}
		case "COMPLETED":
			completedCount[s.AssetID]++
		}
	}

	items := []maintenanceRisk{}
	for _, a := range assets {
		if a.Status == "DISPOSED" {
			continue
		}
		item := maintenanceRisk{
			SubjectType: "asset",
			SubjectID:   a.ID,
			Code:        a.AssetCode,
			Name:        a.Name,
			Factors:     []riskFactor{},
		}

		// Jadwal yang lewat adalah satu-satunya sinyal yang benar-benar
		// menyatakan "perawatan yang seharusnya sudah terjadi belum terjadi".
		// Dibatasi 60 hari supaya jadwal yang terlupakan bertahun-tahun tidak
		// menenggelamkan seluruh daftar.
		if days := overdueDays[a.ID]; days > 0 {
			points := math.Min(float64(days), 60) / 60 * 45
			item.Factors = append(item.Factors, riskFactor{
				Code:         "OVERDUE_MAINTENANCE",
				Label:        "Jadwal maintenance terlewat",
				Contribution: round1(points),
				Detail:       itoa(days) + " hari melewati jadwal",
			})
		}

		if completedCount[a.ID] == 0 {
			item.Factors = append(item.Factors, riskFactor{
				Code:         "NEVER_MAINTAINED",
				Label:        "Belum pernah ada maintenance yang selesai",
				Contribution: 20,
				Detail:       "Tidak ada riwayat perawatan untuk aset ini",
			})
		}

		if a.Status == "MAINTENANCE" {
			item.Factors = append(item.Factors, riskFactor{
				Code:         "IN_MAINTENANCE",
				Label:        "Aset sedang berstatus MAINTENANCE",
				Contribution: 15,
				Detail:       "Sedang tidak bisa dipakai",
			})
		}

		// Sisa umur manfaat -- memakai angka penyusutan yang dibangun di Fase 5.
		// Aset yang nilai bukunya tinggal 10% dari nilai yang bisa disusutkan
		// sudah mendekati akhir umur rencananya, dan biaya perawatannya
		// biasanya naik justru di titik itu.
		if a.UsefulLifeMonths != nil {
			depreciable := a.AcquisitionCost - a.SalvageValue
			remaining := a.BookValue - a.SalvageValue
			if depreciable > 0 && remaining <= depreciable*0.1 {
				item.Factors = append(item.Factors, riskFactor{
					Code:         "END_OF_LIFE",
					Label:        "Mendekati akhir umur manfaat",
					Contribution: 15,
					Detail:       "Nilai buku tinggal " + formatPercent(remaining/depreciable) + " dari nilai yang disusutkan",
				})
			}
		}

		if len(item.Factors) == 0 {
			continue
		}
		item.RiskScore = round1(totalContribution(item.Factors))
		item.RiskLevel = riskLevel(item.RiskScore)
		item.Action = assetAction(item)
		items = append(items, item)
	}
	return items
}

func assetAction(item maintenanceRisk) string {
	switch topFactor(item.Factors) {
	case "OVERDUE_MAINTENANCE":
		return "Kerjakan atau jadwalkan ulang maintenance yang sudah lewat"
	case "NEVER_MAINTAINED":
		return "Buat jadwal maintenance pertama untuk aset ini"
	case "END_OF_LIFE":
		return "Pertimbangkan penggantian: biaya perawatan aset di akhir umur manfaat biasanya melampaui manfaatnya"
	default:
		return "Pastikan perbaikan yang sedang berjalan selesai"
	}
}

func totalContribution(factors []riskFactor) float64 {
	total := 0.0
	for _, f := range factors {
		total += f.Contribution
	}
	// Skor tetap 0-100 supaya bisa dibandingkan antar subjek walau jumlah
	// faktornya berbeda.
	return math.Min(total, 100)
}

func topFactor(factors []riskFactor) string {
	best := ""
	bestPoints := -1.0
	for _, f := range factors {
		if f.Contribution > bestPoints {
			best, bestPoints = f.Code, f.Contribution
		}
	}
	return best
}

// Pembantu format kecil, sengaja lokal: isi field Detail dibaca manusia,
// bukan diparse mesin.
func formatPercent(v float64) string {
	return strconv.FormatFloat(math.Round(v*1000)/10, 'f', -1, 64) + "%"
}

func itoa(v int) string { return strconv.Itoa(v) }

// firstTen memotong "2026-08-01T00:00:00Z" menjadi "2026-08-01" -- tanggal dari
// service lain kadang datang sebagai timestamp penuh.
func firstTen(v string) string {
	if len(v) < 10 {
		return v
	}
	return v[:10]
}
