package etl

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

// Empat fact table terakhir yang belum punya query ringkasan: payroll, perawatan
// aset, sensor IoT, dan penjualan online. Dengan ini SELURUH 16 fact table punya
// endpoint analitiknya sendiri.

func payrollRow(companyID uuid.UUID, period, runStatus string, gross, deduction, net float64, workingDays, presentDays int16) ch.HRPayrollDetailRow {
	return ch.HRPayrollDetailRow{
		DetailID: uuid.New(), PayrollRunID: uuid.New(), CompanyID: companyID,
		Period: period, RunStatus: runStatus, EmployeeID: uuid.New(), EmployeeCode: "EMP-X",
		EmployeeName: "Test", Department: "Umum", BasicSalary: gross, GrossSalary: gross,
		TotalDeduction: deduction, NetSalary: net, WorkingDays: workingDays, PresentDays: presentDays,
	}
}

// Hanya run POSTED yang dihitung -- run DRAFT bisa dihapus dan diproses ulang,
// jadi memasukkannya membuat "biaya gaji bulan ini" berubah-ubah.
func TestPayrollPeriodSummary_PostedOnly(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	rows := []ch.HRPayrollDetailRow{
		payrollRow(companyID, "2026-08", "POSTED", 10_000_000, 1_000_000, 9_000_000, 22, 22),
		payrollRow(companyID, "2026-08", "POSTED", 6_000_000, 500_000, 5_500_000, 22, 11),
		payrollRow(companyID, "2026-08", "DRAFT", 99_000_000, 0, 99_000_000, 22, 22),
	}
	if err := chClient.InsertHRPayrollDetails(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertHRPayrollDetails: %v", err)
	}

	summary, err := chClient.PayrollPeriodSummary(ctx, companyID)
	if err != nil {
		t.Fatalf("PayrollPeriodSummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected 1 periode, got %d (%+v)", len(summary), summary)
	}

	p := summary[0]
	if p.EmployeeCount != 2 {
		t.Errorf("employee_count = %d, want 2 (yang DRAFT tidak ikut)", p.EmployeeCount)
	}
	if !p.TotalNet.Equal(decimal.NewFromInt(14_500_000)) {
		t.Errorf("total_net = %v, want 14.500.000", p.TotalNet)
	}
	// 33 hari hadir dari 44 hari kerja = 75%, dihitung dari TOTAL hari, bukan
	// rata-rata persentase per orang (yang akan memberi (100+50)/2 = 75 juga di
	// contoh ini, tapi berbeda begitu hari kerjanya tidak sama).
	if p.AttendancePct == nil || *p.AttendancePct < 74.9 || *p.AttendancePct > 75.1 {
		t.Errorf("attendance_pct = %v, want 75", p.AttendancePct)
	}
}

func assetRow(companyID uuid.UUID, scheduled string, status string, completed *string, t *testing.T) ch.AssetMaintenanceRow {
	var completedDate *time.Time
	if completed != nil {
		d := mustDate(t, *completed)
		completedDate = &d
	}
	return ch.AssetMaintenanceRow{
		ScheduleID: uuid.New(), CompanyID: companyID, AssetID: uuid.New(),
		AssetCode: "AST-1", AssetName: "Mesin", MaintenanceType: "PREVENTIVE",
		ScheduledDate: mustDate(t, scheduled), CompletedDate: completedDate,
		Status: status, UpdatedAt: time.Now(),
	}
}

// overdue dihitung SAAT QUERY (today()), bukan dibekukan di ETL: keterlambatan
// bertambah dengan sendirinya walau tidak ada sync yang jalan.
func TestAssetMaintenanceSummary_OverdueAndDelay(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	selesai := "2026-08-14" // 4 hari setelah jadwal

	rows := []ch.AssetMaintenanceRow{
		assetRow(companyID, "2026-08-10", "COMPLETED", &selesai, t),
		assetRow(companyID, "2026-08-11", "CANCELLED", nil, t),
		// Jadwal 2020 dan belum selesai -> pasti terlambat kapan pun test ini jalan.
		assetRow(companyID, "2020-01-05", "SCHEDULED", nil, t),
	}
	if err := chClient.InsertAssetMaintenance(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertAssetMaintenance: %v", err)
	}

	summary, err := chClient.AssetMaintenanceSummary(ctx, companyID)
	if err != nil {
		t.Fatalf("AssetMaintenanceSummary: %v", err)
	}
	if len(summary) != 2 {
		t.Fatalf("expected 2 bulan, got %d (%+v)", len(summary), summary)
	}

	lama, agustus := summary[0], summary[1]
	if lama.OverdueCount != 1 {
		t.Errorf("bulan 2020: overdue = %d, want 1", lama.OverdueCount)
	}
	if agustus.CompletedCount != 1 || agustus.CancelledCount != 1 {
		t.Errorf("Agustus: completed/cancelled = %d/%d, want 1/1", agustus.CompletedCount, agustus.CancelledCount)
	}
	// Yang CANCELLED tidak punya completed_date, jadi tidak ikut rata-rata.
	if agustus.AvgDelayDays == nil || *agustus.AvgDelayDays != 4 {
		t.Errorf("avg_delay_days = %v, want 4", agustus.AvgDelayDays)
	}
	if agustus.OverdueCount != 0 {
		t.Errorf("Agustus: yang sudah selesai/dibatalkan tidak boleh terhitung terlambat, got %d", agustus.OverdueCount)
	}
}

func iotRow(companyID uuid.UUID, deviceCode, readingType string, value *float64, recordedAt string) ch.IoTReadingRow {
	ts, _ := time.Parse("2006-01-02T15:04:05Z", recordedAt)
	return ch.IoTReadingRow{
		ReadingID: uuid.New(), CompanyID: companyID, DeviceID: uuid.New(),
		DeviceCode: deviceCode, DeviceType: "SENSOR", ReadingType: readingType,
		ValueNumeric: value, RecordedAt: ts,
	}
}

// Satu device bisa mengirim beberapa jenis pembacaan; merata-ratakan suhu
// bersama kelembapan menghasilkan angka yang tidak berarti apa-apa.
func TestIoTDeviceSummary_GroupsByDeviceAndReadingType(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	v := func(f float64) *float64 { return &f }
	code := "DEV-" + uuid.NewString()[:6]

	rows := []ch.IoTReadingRow{
		iotRow(companyID, code, "TEMPERATURE", v(30), "2026-08-01T08:00:00Z"),
		iotRow(companyID, code, "TEMPERATURE", v(34), "2026-08-01T09:00:00Z"),
		iotRow(companyID, code, "HUMIDITY", v(70), "2026-08-01T09:30:00Z"),
		// Pembacaan teks (mis. status ON/OFF) tidak punya rata-rata -> dibuang.
		iotRow(companyID, code, "STATUS", nil, "2026-08-01T10:00:00Z"),
	}
	if err := chClient.InsertIoTReadings(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertIoTReadings: %v", err)
	}

	summary, err := chClient.IoTDeviceSummary(ctx, companyID)
	if err != nil {
		t.Fatalf("IoTDeviceSummary: %v", err)
	}
	if len(summary) != 2 {
		t.Fatalf("expected 2 kelompok (suhu & kelembapan), got %d (%+v)", len(summary), summary)
	}

	byType := map[string]int{}
	for i, r := range summary {
		byType[r.ReadingType] = i
	}
	suhu := summary[byType["TEMPERATURE"]]
	if suhu.ReadingCount != 2 || *suhu.AvgValue != 32 {
		t.Errorf("suhu: count/avg = %d/%v, want 2/32", suhu.ReadingCount, suhu.AvgValue)
	}
	if !suhu.MinValue.Equal(decimal.NewFromInt(30)) || !suhu.MaxValue.Equal(decimal.NewFromInt(34)) {
		t.Errorf("suhu: min/max = %v/%v, want 30/34", suhu.MinValue, suhu.MaxValue)
	}
	if suhu.LastReadAt == "" {
		t.Error("last_read_at kosong -- pertanyaan pertama tentang sensor adalah 'masih hidup atau tidak'")
	}
	if _, ada := byType["STATUS"]; ada {
		t.Error("pembacaan non-numerik seharusnya tidak ikut diringkas")
	}
}

func ecomRow(companyID, orderID uuid.UUID, status string, qty, amount float64, date string, t *testing.T) ch.EcommerceOrderLineRow {
	return ch.EcommerceOrderLineRow{
		LineID: uuid.New(), OrderID: orderID, CompanyID: companyID,
		OrderNumber: "ORD-1", OrderDate: mustDate(t, date), OrderStatus: status,
		CustomerName: "Pembeli", ProductID: uuid.New(), ProductSKU: "SKU-1", ProductName: "Barang",
		Quantity: qty, UnitPrice: amount / qty, Amount: amount, UpdatedAt: time.Now(),
	}
}

// CANCELLED dibuang (barang tidak pernah dikirim), PENDING tetap dihitung
// (penjualan yang sedang berjalan). Rata-rata dihitung per ORDER, bukan per baris.
func TestEcommerceMonthlySummary_ExcludesCancelledAndAveragesPerOrder(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	orderA, orderB := uuid.New(), uuid.New()

	rows := []ch.EcommerceOrderLineRow{
		ecomRow(companyID, orderA, "PAID", 2, 200_000, "2026-08-02", t),
		ecomRow(companyID, orderA, "PAID", 1, 100_000, "2026-08-02", t), // order yang sama
		ecomRow(companyID, orderB, "PENDING", 1, 300_000, "2026-08-05", t),
		ecomRow(companyID, uuid.New(), "CANCELLED", 5, 999_000, "2026-08-07", t),
	}
	if err := chClient.InsertEcommerceOrderLines(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertEcommerceOrderLines: %v", err)
	}

	summary, err := chClient.EcommerceMonthlySummary(ctx, companyID)
	if err != nil {
		t.Fatalf("EcommerceMonthlySummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected 1 bulan, got %d", len(summary))
	}

	m := summary[0]
	if m.OrderCount != 2 || m.LineCount != 3 {
		t.Errorf("order/line = %d/%d, want 2/3 (CANCELLED dibuang, satu order dua baris)", m.OrderCount, m.LineCount)
	}
	if !m.Revenue.Equal(decimal.NewFromInt(600_000)) {
		t.Errorf("revenue = %v, want 600.000", m.Revenue)
	}
	// 600.000 / 2 order = 300.000 per keranjang (bukan 600.000/3 baris = 200.000).
	if m.AvgOrderSize == nil || *m.AvgOrderSize != 300_000 {
		t.Errorf("avg_order_value = %v, want 300.000", m.AvgOrderSize)
	}
}
