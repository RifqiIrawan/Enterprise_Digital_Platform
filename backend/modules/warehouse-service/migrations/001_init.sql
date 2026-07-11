-- Warehouse Service: product master, warehouse master, stock balance
-- (materialized on-hand quantity per warehouse+product), stock movement
-- ledger, stock transfer antar warehouse/branch, dan stock opname. Mengikuti
-- prinsip platform (README root): setiap data transaksi membawa
-- company_id/branch_id, tidak ada FK fisik lintas database.
--
-- Ini modul pertama yang punya product master tersendiri. Sales/Purchasing
-- masih menyimpan product_name sebagai teks bebas di baris order mereka
-- (lihat migrations 001_init.sql di sales-service/purchasing-service); saat
-- PO RECEIVED atau SO FULFILLED memicu panggilan ke warehouse-service
-- (lihat internal/httpapi/stock_movements.go), produk dicocokkan lewat nama
-- (company_id, name) dan dibuat otomatis di sini kalau belum ada -- tanpa FK
-- fisik ke tabel purchase_order_lines/sales_order_lines di database lain.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE products (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  UUID NOT NULL,
    branch_id   UUID,
    sku         VARCHAR(30) NOT NULL,
    name        VARCHAR(200) NOT NULL,
    unit        VARCHAR(20) NOT NULL DEFAULT 'pcs',
    category    VARCHAR(100),
    cost_price  NUMERIC(15, 2) NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, sku),
    UNIQUE (company_id, name)
);

CREATE INDEX idx_products_company_id ON products (company_id);

CREATE TABLE warehouses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  UUID NOT NULL,
    branch_id   UUID,
    code        VARCHAR(20) NOT NULL,
    name        VARCHAR(200) NOT NULL,
    address     TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_warehouses_company_id ON warehouses (company_id);

-- Saldo stok per gudang per produk, di-maintain transaksional bersamaan
-- dengan insert stock_movements (lihat internal/httpapi) supaya query saldo
-- tidak perlu SUM() dari seluruh ledger tiap request.
CREATE TABLE stock_balances (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    warehouse_id UUID NOT NULL REFERENCES warehouses(id),
    product_id  UUID NOT NULL REFERENCES products(id),
    quantity    NUMERIC(15, 2) NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (warehouse_id, product_id)
);

CREATE INDEX idx_stock_balances_warehouse_id ON stock_balances (warehouse_id);

-- Ledger histori semua pergerakan stok. reference_type/reference_id menunjuk
-- ke entitas pemicu (PURCHASE_ORDER, SALES_ORDER, TRANSFER, OPNAME, MANUAL),
-- tanpa FK fisik karena bisa berasal dari database service lain.
CREATE TABLE stock_movements (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     UUID NOT NULL,
    branch_id      UUID,
    warehouse_id   UUID NOT NULL REFERENCES warehouses(id),
    product_id     UUID NOT NULL REFERENCES products(id),
    movement_type  VARCHAR(10) NOT NULL CHECK (movement_type IN ('IN', 'OUT')),
    quantity       NUMERIC(15, 2) NOT NULL CHECK (quantity > 0),
    reference_type VARCHAR(30) NOT NULL DEFAULT 'MANUAL' CHECK (reference_type IN ('PURCHASE_ORDER', 'SALES_ORDER', 'TRANSFER', 'OPNAME', 'MANUAL')),
    reference_id   UUID,
    notes          TEXT,
    movement_date  DATE NOT NULL DEFAULT CURRENT_DATE,
    actor_user_id  UUID,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_stock_movements_company_id ON stock_movements (company_id);
CREATE INDEX idx_stock_movements_warehouse_id ON stock_movements (warehouse_id, movement_date DESC);
CREATE INDEX idx_stock_movements_product_id ON stock_movements (product_id);

-- Stock transfer: mutasi antar warehouse/branch. Saat CONFIRMED, satu baris
-- transfer menghasilkan dua stock_movements (OUT di from_warehouse, IN di
-- to_warehouse) dengan reference_type=TRANSFER.
CREATE TABLE stock_transfers (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    transfer_number  VARCHAR(30) NOT NULL,
    from_warehouse_id UUID NOT NULL REFERENCES warehouses(id),
    to_warehouse_id  UUID NOT NULL REFERENCES warehouses(id),
    transfer_date    DATE NOT NULL,
    status           VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'CONFIRMED', 'CANCELLED')),
    notes            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, transfer_number)
);

CREATE INDEX idx_stock_transfers_company_id ON stock_transfers (company_id);

CREATE TABLE stock_transfer_lines (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id  UUID NOT NULL REFERENCES stock_transfers(id) ON DELETE CASCADE,
    line_number  SMALLINT NOT NULL,
    product_id   UUID NOT NULL REFERENCES products(id),
    quantity     NUMERIC(15, 2) NOT NULL CHECK (quantity > 0)
);

CREATE INDEX idx_stock_transfer_lines_transfer_id ON stock_transfer_lines (transfer_id);

-- Stock opname: hitung fisik per warehouse. system_quantity di-snapshot dari
-- stock_balances saat baris dibuat (DRAFT); saat POST, selisih
-- (counted_quantity - system_quantity) dicatat sebagai stock_movements
-- ADJUSTMENT (IN kalau lebih, OUT kalau kurang) dan stock_balances
-- diselaraskan langsung ke counted_quantity.
CREATE TABLE stock_opnames (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     UUID NOT NULL,
    warehouse_id   UUID NOT NULL REFERENCES warehouses(id),
    opname_number  VARCHAR(30) NOT NULL,
    opname_date    DATE NOT NULL,
    status         VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'POSTED')),
    notes          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, opname_number)
);

CREATE INDEX idx_stock_opnames_company_id ON stock_opnames (company_id);

CREATE TABLE stock_opname_lines (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    opname_id        UUID NOT NULL REFERENCES stock_opnames(id) ON DELETE CASCADE,
    product_id       UUID NOT NULL REFERENCES products(id),
    system_quantity  NUMERIC(15, 2) NOT NULL DEFAULT 0,
    counted_quantity NUMERIC(15, 2) NOT NULL DEFAULT 0
);

CREATE INDEX idx_stock_opname_lines_opname_id ON stock_opname_lines (opname_id);
