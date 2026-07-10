-- Sales Service: customer master, quotation, dan sales order. Mengikuti
-- prinsip platform (README root): setiap data transaksi membawa
-- company_id/branch_id, tidak ada FK fisik lintas database. Produk belum
-- punya master table tersendiri (belum ada modul Warehouse/Inventory di
-- Fase 2 ini), jadi product_name disimpan sebagai teks bebas di baris
-- quotation/sales order -- simplifikasi yang sama seperti partner_name
-- bebas di finance-service.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE customers (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL,
    branch_id     UUID,
    customer_code VARCHAR(20) NOT NULL,
    name          VARCHAR(200) NOT NULL,
    email         VARCHAR(200),
    phone         VARCHAR(20),
    address       TEXT,
    tax_id        VARCHAR(30), -- NPWP, opsional
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, customer_code)
);

CREATE INDEX idx_customers_company_id ON customers (company_id);

CREATE TABLE quotations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    branch_id        UUID,
    quotation_number VARCHAR(30) NOT NULL,
    customer_id      UUID NOT NULL REFERENCES customers(id),
    quotation_date   DATE NOT NULL,
    valid_until      DATE,
    status           VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'SENT', 'ACCEPTED', 'REJECTED', 'EXPIRED', 'CONVERTED')),
    subtotal_amount  NUMERIC(15, 2) NOT NULL DEFAULT 0,
    tax_amount       NUMERIC(15, 2) NOT NULL DEFAULT 0,
    total_amount     NUMERIC(15, 2) NOT NULL DEFAULT 0,
    notes            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, quotation_number)
);

CREATE INDEX idx_quotations_company_id ON quotations (company_id);
CREATE INDEX idx_quotations_customer_id ON quotations (customer_id);

CREATE TABLE quotation_lines (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quotation_id UUID NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    line_number  SMALLINT NOT NULL,
    product_name VARCHAR(200) NOT NULL,
    description  VARCHAR(255),
    quantity     NUMERIC(12, 2) NOT NULL DEFAULT 1,
    unit_price   NUMERIC(15, 2) NOT NULL DEFAULT 0,
    amount       NUMERIC(15, 2) NOT NULL DEFAULT 0
);

CREATE INDEX idx_quotation_lines_quotation_id ON quotation_lines (quotation_id);

-- Sales order: bisa dibuat langsung atau dikonversi dari quotation ACCEPTED
-- (quotation_id terisi kalau berasal dari konversi). invoice_id terisi
-- setelah SO di-invoice ke finance-service (lintas database, lihat
-- internal/financeclient), tanpa FK fisik.
CREATE TABLE sales_orders (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      UUID NOT NULL,
    branch_id       UUID,
    so_number       VARCHAR(30) NOT NULL,
    customer_id     UUID NOT NULL REFERENCES customers(id),
    quotation_id    UUID REFERENCES quotations(id),
    order_date      DATE NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'CONFIRMED', 'FULFILLED', 'INVOICED', 'CANCELLED')),
    subtotal_amount NUMERIC(15, 2) NOT NULL DEFAULT 0,
    tax_amount      NUMERIC(15, 2) NOT NULL DEFAULT 0,
    total_amount    NUMERIC(15, 2) NOT NULL DEFAULT 0,
    invoice_id      UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, so_number)
);

CREATE INDEX idx_sales_orders_company_id ON sales_orders (company_id);
CREATE INDEX idx_sales_orders_customer_id ON sales_orders (customer_id);
CREATE INDEX idx_sales_orders_status ON sales_orders (company_id, status);

CREATE TABLE sales_order_lines (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sales_order_id UUID NOT NULL REFERENCES sales_orders(id) ON DELETE CASCADE,
    line_number    SMALLINT NOT NULL,
    product_name   VARCHAR(200) NOT NULL,
    description    VARCHAR(255),
    quantity       NUMERIC(12, 2) NOT NULL DEFAULT 1,
    unit_price     NUMERIC(15, 2) NOT NULL DEFAULT 0,
    amount         NUMERIC(15, 2) NOT NULL DEFAULT 0
);

CREATE INDEX idx_sales_order_lines_sales_order_id ON sales_order_lines (sales_order_id);
