package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Vehicles
// ---------------------------------------------------------------------------

func TestCreateVehicle_ValidationAndSuccess(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	t.Run("missing required fields", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/vehicles", map[string]any{"company_id": companyID})
		requireStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("invalid vehicle_type", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/vehicles", map[string]any{
			"company_id": companyID, "vehicle_code": "VHC-X", "plate_number": "B 1 A", "name": "Kapal", "vehicle_type": "SHIP",
		})
		requireStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("negative capacity", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/vehicles", map[string]any{
			"company_id": companyID, "vehicle_code": "VHC-Y", "plate_number": "B 2 A", "name": "Truk", "capacity_kg": -5,
		})
		requireStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("success defaults to VAN and AVAILABLE", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/vehicles", map[string]any{
			"company_id": companyID, "vehicle_code": "VHC-OK", "plate_number": "B 3 A", "name": "Hiace",
		})
		requireStatus(t, resp, http.StatusCreated)
		var v struct {
			Status      string `json:"status"`
			VehicleType string `json:"vehicle_type"`
		}
		resp.decode(t, &v)
		if v.Status != "AVAILABLE" || v.VehicleType != "VAN" {
			t.Errorf("got status=%q type=%q, want AVAILABLE/VAN", v.Status, v.VehicleType)
		}
	})

	t.Run("duplicate vehicle_code in same company", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/vehicles", map[string]any{
			"company_id": companyID, "vehicle_code": "VHC-OK", "plate_number": "B 4 A", "name": "Hiace 2",
		})
		requireStatus(t, resp, http.StatusConflict)
	})
}

func TestListVehicles_ScopedToCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	mustSeedVehicle(t, srv, companyA)
	mustSeedVehicle(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/vehicles?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var vehicles []vehicleFixture
	resp.decode(t, &vehicles)
	if len(vehicles) != 1 {
		t.Fatalf("expected exactly 1 vehicle for company A, got %d", len(vehicles))
	}

	requireStatus(t, getJSON(t, srv.URL+"/vehicles"), http.StatusBadRequest)
}

// TestUpdateVehicle_RejectsManualInUse mengunci keputusan desain: IN_USE hanya
// boleh dipasang oleh dispatch surat jalan, tidak boleh diketik lewat PUT.
func TestUpdateVehicle_RejectsManualInUse(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	v := mustSeedVehicle(t, srv, companyID)

	resp := putJSON(t, srv.URL+"/vehicles/"+v.ID, map[string]any{
		"company_id": companyID, "plate_number": "B 9 Z", "name": "Hiace", "vehicle_type": "VAN", "status": "IN_USE",
	})
	requireStatus(t, resp, http.StatusBadRequest)

	requireStatus(t, putJSON(t, srv.URL+"/vehicles/"+v.ID, map[string]any{
		"company_id": companyID, "plate_number": "B 9 Z", "name": "Hiace", "vehicle_type": "VAN", "status": "MAINTENANCE",
	}), http.StatusOK)
}

func TestUpdateVehicle_NotFoundInOtherCompany(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	v := mustSeedVehicle(t, srv, companyID)

	resp := putJSON(t, srv.URL+"/vehicles/"+v.ID, map[string]any{
		"company_id": newCompanyID(t), "plate_number": "B 9 Z", "name": "Hiace", "vehicle_type": "VAN",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Drivers
// ---------------------------------------------------------------------------

func TestCreateDriver_ValidationAndSuccess(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	requireStatus(t, postJSON(t, srv.URL+"/drivers", map[string]any{"company_id": companyID}), http.StatusBadRequest)

	resp := postJSON(t, srv.URL+"/drivers", map[string]any{
		"company_id": companyID, "driver_code": "DRV-DUP", "name": "Joko",
	})
	requireStatus(t, resp, http.StatusCreated)
	var d driverFixture
	resp.decode(t, &d)
	if d.Status != "AVAILABLE" {
		t.Errorf("driver status = %q, want AVAILABLE", d.Status)
	}

	requireStatus(t, postJSON(t, srv.URL+"/drivers", map[string]any{
		"company_id": companyID, "driver_code": "DRV-DUP", "name": "Joko 2",
	}), http.StatusConflict)
}

func TestUpdateDriver_RejectsManualOnDelivery(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	d := mustSeedDriver(t, srv, companyID)

	requireStatus(t, putJSON(t, srv.URL+"/drivers/"+d.ID, map[string]any{
		"company_id": companyID, "name": "Joko", "status": "ON_DELIVERY",
	}), http.StatusBadRequest)

	requireStatus(t, putJSON(t, srv.URL+"/drivers/"+d.ID, map[string]any{
		"company_id": companyID, "name": "Joko", "status": "INACTIVE",
	}), http.StatusOK)
}

func TestListDrivers_ScopedToCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	mustSeedDriver(t, srv, companyA)
	mustSeedDriver(t, srv, newCompanyID(t))

	resp := getJSON(t, srv.URL+"/drivers?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var drivers []driverFixture
	resp.decode(t, &drivers)
	if len(drivers) != 1 {
		t.Fatalf("expected exactly 1 driver for company A, got %d", len(drivers))
	}
}

// ---------------------------------------------------------------------------
// Delivery orders -- standalone (no e-commerce order)
// ---------------------------------------------------------------------------

func TestCreateDeliveryOrder_Validation(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)

	t.Run("missing vehicle and driver", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/delivery-orders", map[string]any{"company_id": companyID}), http.StatusBadRequest)
	})

	t.Run("recipient details required when no order linked", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
			"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID,
		})
		requireStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("vehicle from another company is rejected", func(t *testing.T) {
		other := newServer(t)
		otherCompany := newCompanyID(t)
		foreignVehicle := mustSeedVehicle(t, other, otherCompany)
		resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
			"company_id": companyID, "vehicle_id": foreignVehicle.ID, "driver_id": d.ID,
			"recipient_name": "Siti", "destination_address": "Jl. A",
		})
		requireStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("bad scheduled_date format", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
			"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID,
			"recipient_name": "Siti", "destination_address": "Jl. A", "scheduled_date": "12-08-2026",
		})
		requireStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("success generates DLV number", func(t *testing.T) {
		del, _, _ := mustSeedDelivery(t, srv, newCompanyID(t))
		if del.Status != "PENDING" {
			t.Errorf("status = %q, want PENDING", del.Status)
		}
		if len(del.DeliveryNumber) < 4 || del.DeliveryNumber[:4] != "DLV-" {
			t.Errorf("delivery_number = %q, want DLV- prefix", del.DeliveryNumber)
		}
		if del.EcommerceOrderID != nil {
			t.Errorf("ecommerce_order_id = %v, want nil for a standalone delivery", *del.EcommerceOrderID)
		}
	})
}

func TestCreateDeliveryOrder_RejectsMaintenanceVehicleAndInactiveDriver(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)

	requireStatus(t, putJSON(t, srv.URL+"/vehicles/"+v.ID, map[string]any{
		"company_id": companyID, "plate_number": "B 1 A", "name": "Hiace", "vehicle_type": "VAN", "status": "MAINTENANCE",
	}), http.StatusOK)

	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID,
		"recipient_name": "Siti", "destination_address": "Jl. A",
	})
	requireStatus(t, resp, http.StatusConflict)

	okVehicle := mustSeedVehicle(t, srv, companyID)
	requireStatus(t, putJSON(t, srv.URL+"/drivers/"+d.ID, map[string]any{
		"company_id": companyID, "name": "Joko", "status": "INACTIVE",
	}), http.StatusOK)

	resp = postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": okVehicle.ID, "driver_id": d.ID,
		"recipient_name": "Siti", "destination_address": "Jl. A",
	})
	requireStatus(t, resp, http.StatusConflict)
}

// TestDeliveryLifecycle_MovesVehicleAndDriverStatus adalah inti modul ini:
// status kendaraan/pengemudi HARUS ikut bergerak mengikuti surat jalan, bukan
// cuma status surat jalannya sendiri yang berubah.
func TestDeliveryLifecycle_MovesVehicleAndDriverStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	del, v, d := mustSeedDelivery(t, srv, companyID)

	if got := fetchVehicleStatus(t, srv, companyID, v.ID); got != "AVAILABLE" {
		t.Fatalf("vehicle before dispatch = %q, want AVAILABLE (PENDING tidak boleh mengunci kendaraan)", got)
	}

	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/dispatch", nil), http.StatusOK)
	if got := fetchVehicleStatus(t, srv, companyID, v.ID); got != "IN_USE" {
		t.Errorf("vehicle after dispatch = %q, want IN_USE", got)
	}
	if got := fetchDriverStatus(t, srv, companyID, d.ID); got != "ON_DELIVERY" {
		t.Errorf("driver after dispatch = %q, want ON_DELIVERY", got)
	}

	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/deliver", nil), http.StatusOK)
	if got := fetchVehicleStatus(t, srv, companyID, v.ID); got != "AVAILABLE" {
		t.Errorf("vehicle after deliver = %q, want AVAILABLE", got)
	}
	if got := fetchDriverStatus(t, srv, companyID, d.ID); got != "AVAILABLE" {
		t.Errorf("driver after deliver = %q, want AVAILABLE", got)
	}
}

func TestDispatch_RejectsVehicleAlreadyInUse(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	first, v, d := mustSeedDelivery(t, srv, companyID)
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+first.ID+"/dispatch", nil), http.StatusOK)

	// Surat jalan kedua boleh DIBUAT dengan kendaraan yang sama (penjadwalan
	// ke depan), tapi tidak boleh diberangkatkan selagi kendaraannya jalan.
	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID,
		"recipient_name": "Andi", "destination_address": "Jl. B",
	})
	requireStatus(t, resp, http.StatusCreated)
	var second deliveryFixture
	resp.decode(t, &second)

	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+second.ID+"/dispatch", nil), http.StatusConflict)
}

func TestDeliveryTransitions_RejectOutOfOrder(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	del, _, _ := mustSeedDelivery(t, srv, companyID)

	// Belum dispatch -> tidak bisa langsung deliver.
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/deliver", nil), http.StatusConflict)

	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/dispatch", nil), http.StatusOK)
	// Sudah dispatch -> tidak bisa dispatch lagi.
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/dispatch", nil), http.StatusConflict)

	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/deliver", nil), http.StatusOK)
	// Sudah selesai -> tidak bisa dibatalkan.
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/cancel", nil), http.StatusConflict)
}

func TestCancelDispatchedDelivery_ReleasesVehicleAndDriver(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	del, v, d := mustSeedDelivery(t, srv, companyID)
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/dispatch", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/cancel", nil), http.StatusOK)

	if got := fetchVehicleStatus(t, srv, companyID, v.ID); got != "AVAILABLE" {
		t.Errorf("vehicle after cancel = %q, want AVAILABLE", got)
	}
	if got := fetchDriverStatus(t, srv, companyID, d.ID); got != "AVAILABLE" {
		t.Errorf("driver after cancel = %q, want AVAILABLE", got)
	}
}

func TestUpdateDeliveryOrder_OnlyWhilePending(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	del, v, d := mustSeedDelivery(t, srv, companyID)

	payload := map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID,
		"recipient_name": "Siti Diperbarui", "destination_address": "Jl. Baru No. 9",
	}
	resp := putJSON(t, srv.URL+"/delivery-orders/"+del.ID, payload)
	requireStatus(t, resp, http.StatusOK)
	var updated deliveryFixture
	resp.decode(t, &updated)
	if updated.RecipientName != "Siti Diperbarui" {
		t.Errorf("recipient_name = %q, want updated value", updated.RecipientName)
	}

	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/dispatch", nil), http.StatusOK)
	requireStatus(t, putJSON(t, srv.URL+"/delivery-orders/"+del.ID, payload), http.StatusConflict)
}

func TestGetDeliveryOrder_NotFound(t *testing.T) {
	srv := newServer(t)
	requireStatus(t, getJSON(t, srv.URL+"/delivery-orders/"+uuid.NewString()), http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Delivery orders -- linked to an e-commerce order (cross-service)
// ---------------------------------------------------------------------------

func happyStub(companyID, orderID string) ecommerceStubConfig {
	return ecommerceStubConfig{
		orderID:      orderID,
		companyID:    companyID,
		orderStatus:  "SHIPPED",
		orderNumber:  "ORD-202608-0001",
		customerName: "Budi Santoso",
		address:      "Jl. Merdeka No. 1",
	}
}

// TestCreateDeliveryOrder_SnapshotsEcommerceOrder membuktikan integrasi lintas
// service benar-benar dipakai: nomor order, nama, dan alamat penerima datang
// dari response ecommerce-service, bukan dari request.
func TestCreateDeliveryOrder_SnapshotsEcommerceOrder(t *testing.T) {
	companyID := newCompanyID(t)
	orderID := uuid.NewString()
	srv, calls := newServerWithEcommerceStub(t, happyStub(companyID, orderID))

	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID,
		"ecommerce_order_id": orderID,
	})
	requireStatus(t, resp, http.StatusCreated)
	var del deliveryFixture
	resp.decode(t, &del)

	if del.RecipientName != "Budi Santoso" {
		t.Errorf("recipient_name = %q, want the customer_name from ecommerce-service", del.RecipientName)
	}
	if del.DestinationAddress != "Jl. Merdeka No. 1" {
		t.Errorf("destination_address = %q, want the shipping_address from ecommerce-service", del.DestinationAddress)
	}
	if del.ReferenceNumber == nil || *del.ReferenceNumber != "ORD-202608-0001" {
		t.Errorf("reference_number = %v, want ORD-202608-0001", del.ReferenceNumber)
	}
	if len(*calls) != 1 || (*calls)[0].method != http.MethodGet {
		t.Errorf("expected exactly 1 GET to ecommerce-service, got %+v", *calls)
	}
}

// Input manual menang atas snapshot -- pengiriman bisa dialihkan ke alamat
// lain daripada yang tertulis di order.
func TestCreateDeliveryOrder_ManualRecipientOverridesSnapshot(t *testing.T) {
	companyID := newCompanyID(t)
	orderID := uuid.NewString()
	srv, _ := newServerWithEcommerceStub(t, happyStub(companyID, orderID))

	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID,
		"ecommerce_order_id":  orderID,
		"recipient_name":      "Titip Tetangga",
		"destination_address": "Jl. Alternatif No. 7",
	})
	requireStatus(t, resp, http.StatusCreated)
	var del deliveryFixture
	resp.decode(t, &del)
	if del.RecipientName != "Titip Tetangga" || del.DestinationAddress != "Jl. Alternatif No. 7" {
		t.Errorf("manual recipient/address should win, got %q / %q", del.RecipientName, del.DestinationAddress)
	}
	// Nomor order tetap di-snapshot walaupun penerima diisi manual.
	if del.ReferenceNumber == nil || *del.ReferenceNumber != "ORD-202608-0001" {
		t.Errorf("reference_number = %v, want the order number even with manual recipient", del.ReferenceNumber)
	}
}

func TestCreateDeliveryOrder_RejectsOrderNotShipped(t *testing.T) {
	companyID := newCompanyID(t)
	orderID := uuid.NewString()
	cfg := happyStub(companyID, orderID)
	cfg.orderStatus = "PAID"
	srv, _ := newServerWithEcommerceStub(t, cfg)

	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID, "ecommerce_order_id": orderID,
	})
	requireStatus(t, resp, http.StatusConflict)
}

// Guard lintas-company: order yang UUID-nya benar tapi milik company lain
// tidak boleh dijadikan surat jalan.
func TestCreateDeliveryOrder_RejectsOrderFromAnotherCompany(t *testing.T) {
	companyID := newCompanyID(t)
	orderID := uuid.NewString()
	cfg := happyStub(newCompanyID(t), orderID) // order milik company LAIN
	srv, _ := newServerWithEcommerceStub(t, cfg)

	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID, "ecommerce_order_id": orderID,
	})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestCreateDeliveryOrder_EcommerceDownIsBadGateway(t *testing.T) {
	companyID := newCompanyID(t)
	cfg := happyStub(companyID, uuid.NewString())
	cfg.getFails = true
	srv, _ := newServerWithEcommerceStub(t, cfg)

	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID, "ecommerce_order_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusBadGateway)
}

// TestDeliverLinkedDelivery_MarksEcommerceOrderDelivered adalah sisi kedua
// integrasi: menyelesaikan surat jalan harus MENGGERAKKAN order di
// ecommerce-service, bukan cuma mengubah status lokal.
func TestDeliverLinkedDelivery_MarksEcommerceOrderDelivered(t *testing.T) {
	companyID := newCompanyID(t)
	orderID := uuid.NewString()
	srv, calls := newServerWithEcommerceStub(t, happyStub(companyID, orderID))

	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)
	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID, "ecommerce_order_id": orderID,
	})
	requireStatus(t, resp, http.StatusCreated)
	var del deliveryFixture
	resp.decode(t, &del)

	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/dispatch", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/deliver", nil), http.StatusOK)

	var delivered []stubCall
	for _, c := range *calls {
		if c.method == http.MethodPost {
			delivered = append(delivered, c)
		}
	}
	if len(delivered) != 1 {
		t.Fatalf("expected exactly 1 POST /deliver to ecommerce-service, got %+v", *calls)
	}
	if delivered[0].path != "/orders/"+orderID+"/deliver" {
		t.Errorf("called %q, want /orders/%s/deliver", delivered[0].path, orderID)
	}
	// X-User-Id diteruskan supaya perubahan status di ecommerce-service
	// tercatat dengan actor yang benar (ecommerce-service tidak memvalidasi
	// JWT, hanya gateway yang melakukannya).
	if delivered[0].actor == "" {
		t.Error("expected X-User-Id to be forwarded to ecommerce-service")
	}
}

// Kalau ecommerce-service menolak, surat jalan TIDAK boleh ikut berubah --
// kalau dibalik urutannya, surat jalan jadi DELIVERED sementara order-nya
// masih SHIPPED dan tidak ada yang membereskan.
func TestDeliverLinkedDelivery_EcommerceFailureLeavesDeliveryDispatched(t *testing.T) {
	companyID := newCompanyID(t)
	orderID := uuid.NewString()
	cfg := happyStub(companyID, orderID)
	cfg.deliverFails = true
	srv, _ := newServerWithEcommerceStub(t, cfg)

	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)
	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id": companyID, "vehicle_id": v.ID, "driver_id": d.ID, "ecommerce_order_id": orderID,
	})
	requireStatus(t, resp, http.StatusCreated)
	var del deliveryFixture
	resp.decode(t, &del)

	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/dispatch", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/deliver", nil), http.StatusBadGateway)

	getResp := getJSON(t, srv.URL+"/delivery-orders/"+del.ID)
	requireStatus(t, getResp, http.StatusOK)
	var after deliveryFixture
	getResp.decode(t, &after)
	if after.Status != "DISPATCHED" {
		t.Errorf("status after failed deliver = %q, want DISPATCHED (nothing should have changed)", after.Status)
	}
	if got := fetchVehicleStatus(t, srv, companyID, v.ID); got != "IN_USE" {
		t.Errorf("vehicle after failed deliver = %q, want IN_USE (kendaraan belum boleh dibebaskan)", got)
	}
}

// Surat jalan tanpa ecommerce_order_id tidak boleh menyentuh
// ecommerce-service sama sekali -- newServer memakai alamat yang tidak bisa
// dihubungi, jadi kalau handler tetap memanggilnya test ini akan gagal.
func TestDeliverStandaloneDelivery_DoesNotCallEcommerce(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	del, _, _ := mustSeedDelivery(t, srv, companyID)

	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/dispatch", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/deliver", nil), http.StatusOK)
}

func TestListDeliveryOrders_FiltersByStatusAndBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	del, _, _ := mustSeedDelivery(t, srv, companyID)
	requireStatus(t, postJSON(t, srv.URL+"/delivery-orders/"+del.ID+"/dispatch", nil), http.StatusOK)
	mustSeedDelivery(t, srv, companyID)

	resp := getJSON(t, srv.URL+"/delivery-orders?company_id="+companyID+"&status=DISPATCHED")
	requireStatus(t, resp, http.StatusOK)
	var dispatched []deliveryFixture
	resp.decode(t, &dispatched)
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 DISPATCHED delivery, got %d", len(dispatched))
	}

	// branch_id filter NULL-inclusive: kedua surat jalan di atas dibuat tanpa
	// branch_id, jadi keduanya tetap ikut terbawa untuk branch manapun (pola
	// yang sama dengan 21 endpoint list lain di platform ini).
	resp = getJSON(t, srv.URL+"/delivery-orders?company_id="+companyID+"&branch_id="+uuid.NewString())
	requireStatus(t, resp, http.StatusOK)
	var all []deliveryFixture
	resp.decode(t, &all)
	if len(all) != 2 {
		t.Errorf("expected both NULL-branch deliveries to be included, got %d", len(all))
	}
}
