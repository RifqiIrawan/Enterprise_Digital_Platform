package etl

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

const fleetSourceTable = "delivery_orders"

// fleetExtractSQL watermark pakai delivery_orders.updated_at. Grain SATU baris
// per surat jalan; kendaraan dan pengemudi di-JOIN untuk denormalisasi kode/
// nama (pola sama seperti fact_ticketing_tickets men-join ticket_categories)
// supaya query analitik "pengiriman per kendaraan" tidak perlu dimensi
// terpisah yang belum ada.
//
// dispatched_at/delivered_at disimpan RAW, bukan dihitung jadi durasi menit --
// alasan yang sama seperti resolved_at/closed_at di fact ticketing: biarkan
// query masa depan yang menentukan definisi durasinya, jangan dibekukan di ETL.
//
// destination_address SENGAJA tidak ikut: teks bebas panjang, tidak punya nilai
// analitik agregat, dan menyalin alamat pelanggan ke data lake tanpa kebutuhan
// nyata bukan default yang baik.
const fleetExtractSQL = `
	SELECT d.id, d.company_id, d.branch_id, d.delivery_number,
	       d.vehicle_id, v.vehicle_code, v.vehicle_type,
	       d.driver_id, dr.driver_code, dr.name AS driver_name,
	       d.ecommerce_order_id, d.reference_number, d.recipient_name,
	       d.scheduled_date, d.status, d.dispatched_at, d.delivered_at, d.cancelled_at,
	       d.created_at, d.updated_at
	FROM delivery_orders d
	JOIN vehicles v ON v.id = d.vehicle_id
	JOIN drivers dr ON dr.id = d.driver_id
	WHERE d.updated_at >= $1
	ORDER BY d.updated_at`

// SyncFleet mengekstrak delivery_orders (di-join ke vehicles + drivers) dari
// fleet-service, lalu load ke fact_fleet_delivery_orders di ClickHouse.
// vehicles/drivers sendiri TIDAK dimodelkan sebagai fact terpisah -- keduanya
// master data tanpa measure numerik, dan atribut yang dibutuhkan sudah ikut
// terdenormalisasi di sini.
func SyncFleet(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, fleetSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get fleet watermark: %w", err)
	}

	rows, err := source.Query(ctx, fleetExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract fleet rows: %w", err)
	}
	defer rows.Close()

	var out []ch.FleetDeliveryOrderRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.FleetDeliveryOrderRow
		if err := rows.Scan(
			&r.DeliveryID, &r.CompanyID, &r.BranchID, &r.DeliveryNumber,
			&r.VehicleID, &r.VehicleCode, &r.VehicleType,
			&r.DriverID, &r.DriverCode, &r.DriverName,
			&r.EcommerceOrderID, &r.ReferenceNumber, &r.RecipientName,
			&r.ScheduledDate, &r.Status, &r.DispatchedAt, &r.DeliveredAt, &r.CancelledAt,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan fleet row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate fleet rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertFleetDeliveryOrders(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load fleet rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, fleetSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", fleetSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, fleetSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance fleet watermark: %w", err)
	}
	return len(out), nil
}
