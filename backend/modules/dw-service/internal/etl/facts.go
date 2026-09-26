package etl

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
	"github.com/enterprise-digital-platform/dw-service/internal/sourcedb"
)

// Fact adalah satu-satunya daftar fact yang dikenal dw-service: nama di data
// lake dan status sync (Name), tabel ClickHouse-nya (Table), database Postgres
// sumbernya (Source), dan cara menuliskan SEMUA barisnya ke Bronze (Backfill).
// Backfill, pemeriksaan hitungan lake-vs-ClickHouse, dan /sync/status semuanya
// membaca daftar ini, jadi fact baru cukup ditambahkan di sini.
type Fact struct {
	Name     string
	Table    string
	Source   func(*sourcedb.Pools) *pgxpool.Pool
	Backfill func(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error)
	// LiveKeys: kunci Silver semua baris yang masih ada di sumber, dari ekstrak
	// yang sama dengan sync (jadi "masih ada" berarti "masih akan disalin sync",
	// mis. production_oee hanya run CLOSED). Dipakai Silver untuk memangkas baris
	// yang sudah dihapus di sumber.
	LiveKeys func(ctx context.Context, source *pgxpool.Pool) (map[string]struct{}, error)
}

var Facts = []Fact{
	{financeSourceTable, "fact_finance_journal_lines", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Finance }, BackfillFinance, LiveKeysFinance},
	{salesSourceTable, "fact_sales_order_lines", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Sales }, BackfillSales, LiveKeysSales},
	{inventorySourceTable, "fact_inventory_movements", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Warehouse }, BackfillInventory, LiveKeysInventory},
	{hrSourceTable, "fact_hr_payroll_details", func(p *sourcedb.Pools) *pgxpool.Pool { return p.HR }, BackfillHR, LiveKeysHR},
	{hrLeaveSourceTable, "fact_hr_leave_requests", func(p *sourcedb.Pools) *pgxpool.Pool { return p.HR }, BackfillHRLeave, LiveKeysHRLeave},
	{hrKPISourceTable, "fact_hr_kpi_reviews", func(p *sourcedb.Pools) *pgxpool.Pool { return p.HR }, BackfillHRKPI, LiveKeysHRKPI},
	{purchasingSourceTable, "fact_purchasing_order_lines", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Purchasing }, BackfillPurchasing, LiveKeysPurchasing},
	{productionSourceTable, "fact_production_work_orders", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Production }, BackfillProduction, LiveKeysProduction},
	{productionOEESourceTable, "fact_production_oee", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Production }, BackfillProductionOEE, LiveKeysProductionOEE},
	{qcSourceTable, "fact_qc_inspections", func(p *sourcedb.Pools) *pgxpool.Pool { return p.QC }, BackfillQC, LiveKeysQC},
	{assetSourceTable, "fact_asset_maintenance", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Asset }, BackfillAsset, LiveKeysAsset},
	{iotSourceTable, "fact_iot_readings", func(p *sourcedb.Pools) *pgxpool.Pool { return p.IoT }, BackfillIoT, LiveKeysIoT},
	{crmSourceTable, "fact_crm_opportunities", func(p *sourcedb.Pools) *pgxpool.Pool { return p.CRM }, BackfillCRM, LiveKeysCRM},
	{ticketingSourceTable, "fact_ticketing_tickets", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Ticketing }, BackfillTicketing, LiveKeysTicketing},
	{ecommerceSourceTable, "fact_ecommerce_order_lines", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Ecommerce }, BackfillEcommerce, LiveKeysEcommerce},
	{fleetSourceTable, "fact_fleet_delivery_orders", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Fleet }, BackfillFleet, LiveKeysFleet},
	{projectSourceTable, "fact_project_timesheets", func(p *sourcedb.Pools) *pgxpool.Pool { return p.Project }, BackfillProject, LiveKeysProject},
}

// BackfillResult adalah hasil backfill satu fact.
type BackfillResult struct {
	Fact  string `json:"fact"`
	Rows  int    `json:"rows_written"`
	Error string `json:"error,omitempty"`
}

// BackfillAll menulis SELURUH isi tiap tabel sumber ke Bronze, mengabaikan
// watermark, tanpa menyentuh ClickHouse maupun watermark-nya. Satu fact yang
// gagal dicatat dan tidak menghentikan yang lain (pola yang sama dengan
// RunSync).
func BackfillAll(ctx context.Context, sources *sourcedb.Pools, lake *datalake.Client) []BackfillResult {
	out := make([]BackfillResult, 0, len(Facts))
	for _, f := range Facts {
		n, err := f.Backfill(ctx, f.Source(sources), lake)
		r := BackfillResult{Fact: f.Name, Rows: n}
		if err != nil {
			r.Error = err.Error()
		}
		out = append(out, r)
	}
	return out
}

// backfill adalah isi bersama semua BackfillX: baca semua baris lewat extract
// milik fact itu (watermark nol), tulis satu berkas ke Bronze.
//
// Kenapa ini menutup celah "baris lama tidak pernah masuk lake": ETL biasa
// hanya membaca baris di atas watermark, jadi baris yang sudah tersalin ke
// ClickHouse SEBELUM lake dipasang tidak pernah dibaca lagi. Backfill membaca
// keadaan Postgres SEKARANG dan menuliskannya dengan syncedAt = sekarang; di
// Silver, versi terbaru menang, jadi tumpang tindih dengan berkas Bronze
// sebelumnya tidak menggandakan apa pun.
//
// Berbeda dari sync biasa, kegagalan menulis ke lake di sini ADALAH galat:
// menulis ke lake satu-satunya tujuan operasi ini.
func backfill[T any](ctx context.Context, lake *datalake.Client, table string, extract func() ([]T, error)) (int, error) {
	if lake == nil {
		return 0, fmt.Errorf("backfill %s: data lake (MinIO) tidak tersedia", table)
	}
	rows, err := extract()
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if err := lake.WriteJSONLines(ctx, table, rows, time.Now()); err != nil {
		return 0, fmt.Errorf("backfill %s: %w", table, err)
	}
	return len(rows), nil
}

// liveKeys mengubah hasil ekstrak (watermark nol = seluruh baris) menjadi
// himpunan kunci Silver. Baris yang kuncinya tidak bisa dihitung membuat
// pemanggilan GAGAL: daftar kunci yang bolong akan memangkas baris yang masih
// ada, dan itu lebih buruk daripada tidak memangkas.
func liveKeys[T any](fact string, extract func() ([]T, error)) (map[string]struct{}, error) {
	rows, err := extract()
	if err != nil {
		return nil, err
	}
	keys := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		k, ok := datalake.RowKey(fact, r)
		if !ok {
			return nil, fmt.Errorf("live keys %s: baris sumber tanpa kunci yang valid", fact)
		}
		keys[k] = struct{}{}
	}
	return keys, nil
}
