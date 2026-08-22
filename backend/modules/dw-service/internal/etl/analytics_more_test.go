package etl

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

// Empat fact table yang datanya sudah lama masuk warehouse tapi belum pernah
// punya query ringkasan: QC, produksi, belanja per supplier, dan tiket.

func mustDate(t *testing.T, v string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", v)
	if err != nil {
		t.Fatalf("parse tanggal %q: %v", v, err)
	}
	return d
}

func qcRow(companyID uuid.UUID, date string, result string, inspected, failed float64, t *testing.T) ch.QCInspectionRow {
	return ch.QCInspectionRow{
		InspectionID: uuid.New(), CompanyID: companyID, InspectionNumber: "QC-" + uuid.NewString()[:6],
		StandardID: uuid.New(), StandardCode: "STD-1", ProductID: uuid.New(), ReferenceType: "NONE",
		InspectedQuantity: inspected, PassedQuantity: inspected - failed, FailedQuantity: failed,
		Result: result, InspectionDate: mustDate(t, date), UpdatedAt: time.Now(),
	}
}

// Cacah hasil dan kuantitas menjawab pertanyaan berbeda, jadi keduanya diuji:
// satu inspeksi PARTIAL atas 1.000 unit bukan hal yang sama dengan satu FAIL
// atas 2 unit.
func TestQCMonthlySummary_CountsAndQuantities(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	rows := []ch.QCInspectionRow{
		qcRow(companyID, "2026-08-03", "PASS", 100, 0, t),
		qcRow(companyID, "2026-08-10", "PARTIAL", 1000, 50, t),
		qcRow(companyID, "2026-08-20", "FAIL", 2, 2, t),
		qcRow(companyID, "2026-09-01", "PASS", 10, 0, t),
	}
	if err := chClient.InsertQCInspections(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertQCInspections: %v", err)
	}

	summary, err := chClient.QCMonthlySummary(ctx, companyID)
	if err != nil {
		t.Fatalf("QCMonthlySummary: %v", err)
	}
	if len(summary) != 2 {
		t.Fatalf("expected 2 bulan, got %d (%+v)", len(summary), summary)
	}

	agustus := summary[0]
	if agustus.InspectionsQty != 3 || agustus.PassCount != 1 || agustus.FailCount != 1 || agustus.PartialCount != 1 {
		t.Errorf("cacah hasil tidak sesuai: %+v", agustus)
	}
	if !agustus.InspectedQty.Equal(decimal.NewFromInt(1102)) {
		t.Errorf("inspected_quantity = %v, want 1102", agustus.InspectedQty)
	}
	if !agustus.FailedQty.Equal(decimal.NewFromInt(52)) {
		t.Errorf("failed_quantity = %v, want 52", agustus.FailedQty)
	}
	// 52/1102 = 4,719...% -- dihitung dari kuantitas, bukan dari cacah inspeksi
	// (yang akan memberi 2/3 = 66%).
	if agustus.DefectRatePct == nil {
		t.Fatal("defect_rate_pct nil padahal ada inspeksi")
	}
	if got := *agustus.DefectRatePct; got < 4.7 || got > 4.8 {
		t.Errorf("defect_rate_pct = %v, want ~4.72", got)
	}
}

func TestQCMonthlySummary_NoDataReturnsEmpty(t *testing.T) {
	summary, err := chClient.QCMonthlySummary(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("QCMonthlySummary: %v", err)
	}
	if len(summary) != 0 {
		t.Fatalf("expected kosong, got %d", len(summary))
	}
}

func productionRow(companyID uuid.UUID, date, status string, planned float64, produced *float64, t *testing.T) ch.ProductionWorkOrderRow {
	return ch.ProductionWorkOrderRow{
		WOID: uuid.New(), CompanyID: companyID, WONumber: "WO-" + uuid.NewString()[:6],
		BOMID: uuid.New(), ProductID: uuid.New(), WarehouseID: uuid.New(),
		QuantityPlanned: planned, QuantityProduced: produced, Status: status,
		PlannedStartDate: mustDate(t, date), UpdatedAt: time.Now(),
	}
}

// Realisasi hanya dari WO COMPLETED; rencana dari SELURUH WO bulan itu.
func TestProductionMonthlySummary_RealisationCountsCompletedOnly(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	done80 := 80.0
	inProgress30 := 30.0

	rows := []ch.ProductionWorkOrderRow{
		productionRow(companyID, "2026-08-01", "COMPLETED", 100, &done80, t),
		productionRow(companyID, "2026-08-05", "IN_PROGRESS", 100, &inProgress30, t),
		productionRow(companyID, "2026-08-09", "DRAFT", 50, nil, t),
	}
	if err := chClient.InsertProductionWorkOrders(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertProductionWorkOrders: %v", err)
	}

	summary, err := chClient.ProductionMonthlySummary(ctx, companyID)
	if err != nil {
		t.Fatalf("ProductionMonthlySummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected 1 bulan, got %d", len(summary))
	}

	p := summary[0]
	if p.WorkOrderCount != 3 || p.CompletedCount != 1 {
		t.Errorf("cacah WO = %d/%d, want 3/1", p.WorkOrderCount, p.CompletedCount)
	}
	if !p.QuantityPlanned.Equal(decimal.NewFromInt(250)) {
		t.Errorf("quantity_planned = %v, want 250 (semua WO)", p.QuantityPlanned)
	}
	// 30 dari WO yang masih IN_PROGRESS TIDAK ikut -- angkanya belum final.
	if !p.QuantityDone.Equal(decimal.NewFromInt(80)) {
		t.Errorf("quantity_produced = %v, want 80 (hanya COMPLETED)", p.QuantityDone)
	}
	if p.AchievementPct == nil || *p.AchievementPct < 31.9 || *p.AchievementPct > 32.1 {
		t.Errorf("achievement_pct = %v, want 32 (80/250)", p.AchievementPct)
	}
}

func purchasingRow(companyID uuid.UUID, poID uuid.UUID, supplierCode, status string, amount float64, t *testing.T) ch.PurchasingOrderLineRow {
	return ch.PurchasingOrderLineRow{
		LineID: uuid.New(), PurchaseOrderID: poID, CompanyID: companyID,
		PONumber: "PO-" + supplierCode, OrderDate: mustDate(t, "2026-08-01"), OrderStatus: status,
		SupplierID: uuid.New(), SupplierCode: supplierCode, SupplierName: "Supplier " + supplierCode,
		ProductName: "Barang", Quantity: 1, UnitPrice: amount, Amount: amount, UpdatedAt: time.Now(),
	}
}

// Hanya PO yang benar-benar diterima/ditagih yang dihitung sebagai belanja, dan
// satu PO dengan banyak baris tetap satu order.
func TestPurchasingSupplierSummary_CountsReceivedOnly(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	poA := uuid.New()

	rows := []ch.PurchasingOrderLineRow{
		purchasingRow(companyID, poA, "SUP-A", "RECEIVED", 1000, t),
		purchasingRow(companyID, poA, "SUP-A", "RECEIVED", 500, t), // PO yang sama, baris kedua
		purchasingRow(companyID, uuid.New(), "SUP-A", "INVOICED", 250, t),
		purchasingRow(companyID, uuid.New(), "SUP-B", "RECEIVED", 900, t),
		// Belum jadi belanja -- masih rencana.
		purchasingRow(companyID, uuid.New(), "SUP-A", "DRAFT", 9999, t),
		purchasingRow(companyID, uuid.New(), "SUP-B", "CONFIRMED", 8888, t),
	}
	if err := chClient.InsertPurchasingOrderLines(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertPurchasingOrderLines: %v", err)
	}

	summary, err := chClient.PurchasingSupplierSummary(ctx, companyID)
	if err != nil {
		t.Fatalf("PurchasingSupplierSummary: %v", err)
	}
	if len(summary) != 2 {
		t.Fatalf("expected 2 supplier, got %d (%+v)", len(summary), summary)
	}

	// ORDER BY total_spend DESC -- SUP-A (1750) di atas SUP-B (900).
	a := summary[0]
	if a.SupplierCode != "SUP-A" {
		t.Fatalf("expected SUP-A di urutan pertama, got %s", a.SupplierCode)
	}
	if !a.TotalSpend.Equal(decimal.NewFromInt(1750)) {
		t.Errorf("total_spend SUP-A = %v, want 1750 (DRAFT tidak ikut)", a.TotalSpend)
	}
	if a.LineCount != 3 {
		t.Errorf("line_count SUP-A = %d, want 3", a.LineCount)
	}
	// 2 PO, bukan 3 baris.
	if a.OrderCount != 2 {
		t.Errorf("order_count SUP-A = %d, want 2 (satu PO dua baris tetap satu order)", a.OrderCount)
	}
}

func ticketRow(companyID uuid.UUID, created string, priority string, resolvedAfterMinutes int) ch.TicketingTicketRow {
	createdAt, _ := time.Parse("2006-01-02T15:04:05Z", created)
	var resolvedAt *time.Time
	if resolvedAfterMinutes > 0 {
		r := createdAt.Add(time.Duration(resolvedAfterMinutes) * time.Minute)
		resolvedAt = &r
	}
	return ch.TicketingTicketRow{
		TicketID: uuid.New(), CompanyID: companyID, TicketNumber: "TKT-" + uuid.NewString()[:6],
		CategoryID: uuid.New(), CategoryName: "Umum", Subject: "Uji", Priority: priority,
		Status: "OPEN", RequesterName: "Pemohon", CreatedAt: createdAt, ResolvedAt: resolvedAt,
		UpdatedAt: createdAt,
	}
}

// Lama penyelesaian dihitung dalam menit lalu dibagi 60: tiket yang selesai 45
// menit tidak boleh terbaca 0 jam.
func TestTicketingMonthlySummary_ResolveHoursKeepsMinutes(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	rows := []ch.TicketingTicketRow{
		ticketRow(companyID, "2026-08-01T08:00:00Z", "URGENT", 45),
		ticketRow(companyID, "2026-08-02T08:00:00Z", "LOW", 135),
		ticketRow(companyID, "2026-08-03T08:00:00Z", "LOW", 0), // belum selesai
	}
	if err := chClient.InsertTicketingTickets(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertTicketingTickets: %v", err)
	}

	summary, err := chClient.TicketingMonthlySummary(ctx, companyID)
	if err != nil {
		t.Fatalf("TicketingMonthlySummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected 1 bulan, got %d", len(summary))
	}

	m := summary[0]
	if m.TicketCount != 3 || m.ResolvedCount != 2 || m.OpenCount != 1 {
		t.Errorf("cacah tiket = %d/%d/%d, want 3/2/1", m.TicketCount, m.ResolvedCount, m.OpenCount)
	}
	if m.UrgentCount != 1 {
		t.Errorf("urgent_count = %d, want 1", m.UrgentCount)
	}
	if m.AvgResolveHours == nil {
		t.Fatal("avg_resolve_hours nil padahal ada tiket selesai")
	}
	// (0,75 + 2,25) / 2 = 1,5 jam. Kalau dateDiff('hour') dipakai, hasilnya 1.
	if got := *m.AvgResolveHours; got < 1.49 || got > 1.51 {
		t.Errorf("avg_resolve_hours = %v, want 1.5", got)
	}
}

// Bulan yang seluruh tiketnya belum selesai tidak punya rata-rata.
func TestTicketingMonthlySummary_AvgIsNullWhenNothingResolved(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	if err := chClient.InsertTicketingTickets(ctx, []ch.TicketingTicketRow{
		ticketRow(companyID, "2026-09-01T08:00:00Z", "LOW", 0),
	}, time.Now()); err != nil {
		t.Fatalf("InsertTicketingTickets: %v", err)
	}

	summary, err := chClient.TicketingMonthlySummary(ctx, companyID)
	if err != nil {
		t.Fatalf("TicketingMonthlySummary: %v", err)
	}
	if len(summary) != 1 || summary[0].AvgResolveHours != nil {
		t.Fatalf("expected avg nil, got %+v", summary)
	}
}
