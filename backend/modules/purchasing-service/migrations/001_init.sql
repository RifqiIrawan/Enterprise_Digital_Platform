-- Purchasing Service: supplier master, purchase requisition (PR), dan
-- purchase order (PO). Mirror dari sales-service di sisi AP: PR belum punya
-- supplier (permintaan internal), supplier baru ditentukan saat konversi PR
-- -> PO. Mengikuti prinsip platform (README root): setiap data transaksi
-- membawa company_id/branch_id, tidak ada FK fisik lintas database. Produk
-- belum punya master table tersendiri, jadi product_name disimpan sebagai
-- teks bebas di baris PR/PO -- simplifikasi yang sama seperti sales-service.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE suppliers (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL,
    branch_id     UUID,
    supplier_code VARCHAR(20) NOT NULL,
    name          VARCHAR(200) NOT NULL,
    email         VARCHAR(200),
    phone         VARCHAR(20),
    address       TEXT,
    tax_id        VARCHAR(30), -- NPWP, opsional
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, supplier_code)
);

CREATE INDEX idx_suppliers_company_id ON suppliers (company_id);

-- Purchase Requisition: permintaan internal, belum terikat supplier
-- tertentu (dipilih saat konversi ke PO).
CREATE TABLE purchase_requisitions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      UUID NOT NULL,
    branch_id       UUID,
    pr_number       VARCHAR(30) NOT NULL,
    requested_by    VARCHAR(200), -- nama/departemen pemohon, teks bebas
    pr_date         DATE NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'SUBMITTED', 'APPROVED', 'REJECTED', 'CONVERTED')),
    subtotal_amount NUMERIC(15, 2) NOT NULL DEFAULT 0, -- estimasi, bukan harga final
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, pr_number)
);

CREATE INDEX idx_purchase_requisitions_company_id ON purchase_requisitions (company_id);

CREATE TABLE purchase_requisition_lines (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requisition_id  UUID NOT NULL REFERENCES purchase_requisitions(id) ON DELETE CASCADE,
    line_number     SMALLINT NOT NULL,
    product_name    VARCHAR(200) NOT NULL,
    description     VARCHAR(255),
    quantity        NUMERIC(12, 2) NOT NULL DEFAULT 1,
    estimated_price NUMERIC(15, 2) NOT NULL DEFAULT 0,
    amount          NUMERIC(15, 2) NOT NULL DEFAULT 0
);

CREATE INDEX idx_pr_lines_requisition_id ON purchase_requisition_lines (requisition_id);

-- Purchase Order: bisa dibuat langsung atau dikonversi dari PR APPROVED
-- (requisition_id terisi kalau berasal dari konversi). invoice_id terisi
-- setelah PO di-invoice ke finance-service (lintas database, lihat
-- internal/financeclient), tanpa FK fisik.
CREATE TABLE purchase_orders (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      UUID NOT NULL,
    branch_id       UUID,
    po_number       VARCHAR(30) NOT NULL,
    supplier_id     UUID NOT NULL REFERENCES suppliers(id),
    requisition_id  UUID REFERENCES purchase_requisitions(id),
    order_date      DATE NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'CONFIRMED', 'RECEIVED', 'INVOICED', 'CANCELLED')),
    subtotal_amount NUMERIC(15, 2) NOT NULL DEFAULT 0,
    tax_amount      NUMERIC(15, 2) NOT NULL DEFAULT 0,
    total_amount    NUMERIC(15, 2) NOT NULL DEFAULT 0,
    invoice_id      UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, po_number)
);

CREATE INDEX idx_purchase_orders_company_id ON purchase_orders (company_id);
CREATE INDEX idx_purchase_orders_supplier_id ON purchase_orders (supplier_id);
CREATE INDEX idx_purchase_orders_status ON purchase_orders (company_id, status);

CREATE TABLE purchase_order_lines (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_order_id UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    line_number     SMALLINT NOT NULL,
    product_name    VARCHAR(200) NOT NULL,
    description     VARCHAR(255),
    quantity        NUMERIC(12, 2) NOT NULL DEFAULT 1,
    unit_price      NUMERIC(15, 2) NOT NULL DEFAULT 0,
    amount          NUMERIC(15, 2) NOT NULL DEFAULT 0
);

CREATE INDEX idx_po_lines_purchase_order_id ON purchase_order_lines (purchase_order_id);
