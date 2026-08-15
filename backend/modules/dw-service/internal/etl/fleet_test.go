package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func mustSeedDelivery(t *testing.T, companyID uuid.UUID, status string) (deliveryID uuid.UUID, vehicleCode, driverName string) {
	t.Helper()
	ctx := context.Background()

	vehicleCode = "VHC-" + uuid.NewString()[:8]
	var vehicleID uuid.UUID
	if err := sourcePool.QueryRow(ctx,
		`INSERT INTO vehicles (company_id, vehicle_code, plate_number, name, vehicle_type)
		 VALUES ($1, $2, 'B 1234 XYZ', 'Toyota Hiace', 'VAN') RETURNING id`,
		companyID, vehicleCode,
	).Scan(&vehicleID); err != nil {
		t.Fatalf("seed vehicle: %v", err)
	}

	driverName = "Joko Susilo"
	var driverID uuid.UUID
	if err := sourcePool.QueryRow(ctx,
		`INSERT INTO drivers (company_id, driver_code, name) VALUES ($1, $2, $3) RETURNING id`,
		companyID, "DRV-"+uuid.NewString()[:8], driverName,
	).Scan(&driverID); err != nil {
		t.Fatalf("seed driver: %v", err)
	}

	// $5 dipakai dua kali (nilai kolom + di dalam CASE), jadi butuh cast
	// eksplisit -- tanpa itu Postgres menyimpulkan tipe berbeda untuk parameter
	// yang sama dan menolak query dengan SQLSTATE 42P08. Jebakan yang sama
	// persis seperti UPDATE tasks di project-service.
	if err := sourcePool.QueryRow(ctx, `
		INSERT INTO delivery_orders (company_id, delivery_number, vehicle_id, driver_id, recipient_name, status, dispatched_at)
		VALUES ($1, $2, $3, $4, 'Siti Aminah', $5::varchar, CASE WHEN $5::varchar = 'PENDING' THEN NULL ELSE now() END)
		RETURNING id`,
		companyID, "DLV-"+uuid.NewString()[:8], vehicleID, driverID, status,
	).Scan(&deliveryID); err != nil {
		t.Fatalf("seed delivery order: %v", err)
	}
	return deliveryID, vehicleCode, driverName
}

func TestSyncFleet_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	deliveryID, vehicleCode, driverName := mustSeedDelivery(t, companyID, "DELIVERED")

	n, err := SyncFleet(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncFleet: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	// vehicle_code dan driver_name berasal dari tabel LAIN (JOIN), jadi
	// keduanya sekaligus membuktikan join-nya benar -- bukan cuma barisnya ada.
	var gotVehicleCode, gotDriverName, gotStatus string
	row := chClient.QueryRow(context.Background(),
		"SELECT vehicle_code, driver_name, status FROM fact_fleet_delivery_orders FINAL WHERE delivery_id = ?", deliveryID)
	if err := row.Scan(&gotVehicleCode, &gotDriverName, &gotStatus); err != nil {
		t.Fatalf("query synced fleet row: %v", err)
	}
	if gotVehicleCode != vehicleCode {
		t.Errorf("vehicle_code = %q, want %q", gotVehicleCode, vehicleCode)
	}
	if gotDriverName != driverName {
		t.Errorf("driver_name = %q, want %q", gotDriverName, driverName)
	}
	if gotStatus != "DELIVERED" {
		t.Errorf("status = %q, want DELIVERED", gotStatus)
	}
}

// Surat jalan yang belum berangkat tetap ikut ter-extract, dengan
// dispatched_at NULL -- kalau kolom nullable-nya salah ditangani, scan-nya
// yang akan gagal di sini (pelajaran bug scan-NULL dari sesi crm-service).
func TestSyncFleet_HandlesNullTransitionTimestamps(t *testing.T) {
	companyID := uuid.New()
	deliveryID, _, _ := mustSeedDelivery(t, companyID, "PENDING")

	if _, err := SyncFleet(context.Background(), sourcePool, chClient, nil); err != nil {
		t.Fatalf("SyncFleet: %v", err)
	}

	var dispatchedAtIsNull uint8
	row := chClient.QueryRow(context.Background(),
		"SELECT isNull(dispatched_at) FROM fact_fleet_delivery_orders FINAL WHERE delivery_id = ?", deliveryID)
	if err := row.Scan(&dispatchedAtIsNull); err != nil {
		t.Fatalf("query synced fleet row: %v", err)
	}
	if dispatchedAtIsNull != 1 {
		t.Error("dispatched_at should still be NULL for a PENDING delivery order")
	}
}
