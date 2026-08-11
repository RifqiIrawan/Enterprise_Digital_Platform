-- Fleet Service: armada kendaraan, pengemudi, dan surat jalan (delivery
-- order). Mengikuti prinsip platform (README root): setiap data transaksi
-- membawa company_id/branch_id, tidak ada FK fisik lintas database.
--
-- Delivery order OPSIONAL terhubung ke order e-commerce lewat
-- ecommerce_order_id (nullable UUID, tanpa FK karena beda database/service).
-- Kalau terisi, fleet-service memanggil ecommerce-service saat delivery
-- dibuat untuk mengambil nomor order + nama/alamat penerima sebagai SNAPSHOT
-- (reference_number/recipient_name/destination_address di bawah), bukan
-- live-lookup -- pola snapshot yang sama seperti order_items menyimpan
-- product_sku/product_name dari warehouse-service di ecommerce-service.
-- Delivery TANPA ecommerce_order_id tetap sah: pengiriman internal (antar
-- gudang, kurir dokumen) tidak selalu berasal dari order online.
--
-- vehicles.status dan drivers.status BUKAN data yang diinput bebas saat
-- delivery berjalan -- keduanya digerakkan otomatis oleh lifecycle delivery
-- order (DISPATCHED menandai kendaraan IN_USE + pengemudi ON_DELIVERY,
-- DELIVERED/CANCELLED mengembalikan keduanya ke AVAILABLE), semuanya dalam
-- satu transaksi yang sama dengan perubahan status delivery-nya. Ini
-- satu-satunya business logic lintas-entitas di modul ini.
--
-- Status flow delivery SEMUA lewat endpoint transisi khusus
-- (dispatch/deliver/cancel), tidak ada PUT generik untuk status -- pola
-- ecommerce-service (pay/ship/deliver/cancel) dan sales_orders, bukan pola
-- tickets (PUT bebas). PUT /delivery-orders/{id} cuma untuk field non-status
-- dan hanya selagi PENDING.
--
-- delivery_number di-generate lewat helper nextSequence (disalin dari
-- ecommerce-service/sales-service, bukan diimpor lintas modul).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE vehicles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     UUID NOT NULL,
    branch_id      UUID,
    vehicle_code   VARCHAR(30) NOT NULL,
    plate_number   VARCHAR(20) NOT NULL,
    name           VARCHAR(200) NOT NULL,
    vehicle_type   VARCHAR(20) NOT NULL DEFAULT 'VAN'
        CHECK (vehicle_type IN ('MOTORCYCLE', 'VAN', 'TRUCK')),
    capacity_kg    NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (capacity_kg >= 0),
    status         VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE'
        CHECK (status IN ('AVAILABLE', 'IN_USE', 'MAINTENANCE')),
    notes          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, vehicle_code)
);

CREATE INDEX idx_vehicles_company_id ON vehicles (company_id);
CREATE INDEX idx_vehicles_status ON vehicles (company_id, status);

CREATE TABLE drivers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      UUID NOT NULL,
    branch_id       UUID,
    driver_code     VARCHAR(30) NOT NULL,
    name            VARCHAR(200) NOT NULL,
    phone           VARCHAR(30),
    license_number  VARCHAR(50),
    status          VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE'
        CHECK (status IN ('AVAILABLE', 'ON_DELIVERY', 'INACTIVE')),
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, driver_code)
);

CREATE INDEX idx_drivers_company_id ON drivers (company_id);
CREATE INDEX idx_drivers_status ON drivers (company_id, status);

CREATE TABLE delivery_orders (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          UUID NOT NULL,
    branch_id           UUID,
    delivery_number     VARCHAR(30) NOT NULL,
    vehicle_id          UUID NOT NULL REFERENCES vehicles(id),
    driver_id           UUID NOT NULL REFERENCES drivers(id),
    -- ecommerce-service order, tanpa FK (lintas service/database). NULL =
    -- pengiriman yang tidak berasal dari order online.
    ecommerce_order_id  UUID,
    reference_number    VARCHAR(30),
    recipient_name      VARCHAR(200) NOT NULL,
    recipient_phone     VARCHAR(30),
    destination_address TEXT NOT NULL,
    scheduled_date      DATE NOT NULL DEFAULT CURRENT_DATE,
    status              VARCHAR(20) NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'DISPATCHED', 'DELIVERED', 'CANCELLED')),
    dispatched_at       TIMESTAMPTZ,
    delivered_at        TIMESTAMPTZ,
    cancelled_at        TIMESTAMPTZ,
    notes               TEXT,
    created_by_user_id  UUID, -- dari header X-User-Id, tanpa FK (lintas service)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, delivery_number)
);

CREATE INDEX idx_delivery_orders_company_id ON delivery_orders (company_id);
CREATE INDEX idx_delivery_orders_status ON delivery_orders (company_id, status);
CREATE INDEX idx_delivery_orders_vehicle_id ON delivery_orders (vehicle_id);
CREATE INDEX idx_delivery_orders_driver_id ON delivery_orders (driver_id);
