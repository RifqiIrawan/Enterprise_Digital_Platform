-- Production Service: Bill of Material (BOM) dan Work Order. Mengikuti
-- prinsip platform (README root): setiap data transaksi membawa
-- company_id/branch_id, tidak ada FK fisik lintas database.
--
-- product_id (produk jadi di BOM/work order) dan component_product_id
-- (komponen/bahan baku di bom_lines/work_order_lines) menunjuk ke tabel
-- `products` milik warehouse-service, tanpa FK fisik karena beda database --
-- production-service percaya product_id yang dikirim frontend (yang
-- mengambil daftar produk langsung dari GET /api/warehouse/products).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE bill_of_materials (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  UUID NOT NULL,
    branch_id   UUID,
    bom_code    VARCHAR(30) NOT NULL,
    name        VARCHAR(200) NOT NULL,
    product_id  UUID NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, bom_code)
);

CREATE INDEX idx_bom_company_id ON bill_of_materials (company_id);

CREATE TABLE bom_lines (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bom_id              UUID NOT NULL REFERENCES bill_of_materials(id) ON DELETE CASCADE,
    line_number         SMALLINT NOT NULL,
    component_product_id UUID NOT NULL,
    quantity_per_unit   NUMERIC(15, 4) NOT NULL CHECK (quantity_per_unit > 0)
);

CREATE INDEX idx_bom_lines_bom_id ON bom_lines (bom_id);

-- Work order: satu run produksi dari sebuah BOM. work_order_lines
-- men-snapshot bom_lines * quantity_planned saat work order dibuat, supaya
-- perubahan BOM di kemudian hari tidak mengubah work order yang sudah
-- berjalan. Saat COMPLETED, tiap baris dikonsumsi (stock OUT di
-- warehouse_id) dan produk jadi ditambahkan (stock IN) sebanyak
-- quantity_produced -- lihat internal/httpapi/work_orders.go.
CREATE TABLE work_orders (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        UUID NOT NULL,
    branch_id         UUID,
    wo_number         VARCHAR(30) NOT NULL,
    bom_id            UUID NOT NULL REFERENCES bill_of_materials(id),
    product_id        UUID NOT NULL,
    warehouse_id      UUID NOT NULL,
    quantity_planned  NUMERIC(15, 2) NOT NULL CHECK (quantity_planned > 0),
    quantity_produced NUMERIC(15, 2),
    status            VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED')),
    planned_start_date DATE NOT NULL,
    planned_end_date  DATE,
    notes             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, wo_number)
);

CREATE INDEX idx_work_orders_company_id ON work_orders (company_id);
CREATE INDEX idx_work_orders_status ON work_orders (company_id, status);
CREATE INDEX idx_work_orders_planned_start_date ON work_orders (company_id, planned_start_date);

CREATE TABLE work_order_lines (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    work_order_id        UUID NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    line_number          SMALLINT NOT NULL,
    component_product_id UUID NOT NULL,
    quantity_required    NUMERIC(15, 4) NOT NULL CHECK (quantity_required > 0)
);

CREATE INDEX idx_work_order_lines_work_order_id ON work_order_lines (work_order_id);
