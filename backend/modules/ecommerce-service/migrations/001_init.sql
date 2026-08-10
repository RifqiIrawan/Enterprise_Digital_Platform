-- E-Commerce Service: order (checkout) dan order_items (baris keranjang).
-- Mengikuti prinsip platform (README root): setiap data transaksi membawa
-- company_id/branch_id, tidak ada FK fisik lintas database.
--
-- Katalog produk SENGAJA TIDAK diduplikasi di sini -- order_items.product_id
-- menunjuk langsung ke product warehouse-service (tanpa FK, lintas service),
-- frontend mengambil daftar produk lewat GET /api/warehouse/products untuk
-- mengisi dropdown pemilihan produk saat checkout. product_sku/product_name/
-- unit_price di order_items adalah SNAPSHOT pada saat order dibuat (harga
-- jual TIDAK ada di warehouse-service, cuma cost_price -- unit_price di sini
-- diinput manual sama seperti sales_order_lines.unit_price di sales-service),
-- bukan live-lookup, supaya riwayat order tidak berubah kalau harga produk
-- berubah di kemudian hari.
--
-- customer_name/customer_email standalone (bukan di-link ke CRM
-- Account/Contact), sama seperti keputusan tickets.requester_name/email di
-- ticketing-service -- menghindari pola soft-reference lintas service baru.
--
-- Status flow SEMUA lewat endpoint transisi khusus (pay/ship/deliver/cancel),
-- tidak ada PUT generik untuk status -- pola sales_orders (confirm/fulfill),
-- bukan pola tickets (PUT bebas + /close terminal). PUT /orders/{id} cuma
-- untuk field non-status (customer_name/email/shipping_address/notes),
-- dan hanya diizinkan selagi status PENDING.
--
-- order_number di-generate lewat helper nextSequence (disalin dari
-- sales-service/crm-service/ticketing-service, bukan diimpor lintas modul).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE orders (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id         UUID NOT NULL,
    branch_id          UUID,
    order_number       VARCHAR(30) NOT NULL,
    customer_name      VARCHAR(200) NOT NULL,
    customer_email     VARCHAR(200),
    shipping_address   TEXT,
    status             VARCHAR(20) NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'PAID', 'SHIPPED', 'DELIVERED', 'CANCELLED')),
    order_date         DATE NOT NULL DEFAULT CURRENT_DATE,
    total_amount       NUMERIC(18,2) NOT NULL DEFAULT 0,
    notes              TEXT,
    placed_by_user_id  UUID, -- dari header X-User-Id, tanpa FK (lintas service)
    paid_at            TIMESTAMPTZ,
    shipped_at         TIMESTAMPTZ,
    delivered_at       TIMESTAMPTZ,
    cancelled_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, order_number)
);

CREATE INDEX idx_orders_company_id ON orders (company_id);
CREATE INDEX idx_orders_status ON orders (company_id, status);

CREATE TABLE order_items (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL,
    branch_id     UUID,
    order_id      UUID NOT NULL REFERENCES orders(id),
    product_id    UUID NOT NULL, -- warehouse-service product, tanpa FK (lintas service)
    product_sku   VARCHAR(50) NOT NULL,
    product_name  VARCHAR(200) NOT NULL,
    unit_price    NUMERIC(18,2) NOT NULL CHECK (unit_price >= 0),
    quantity      NUMERIC(18,3) NOT NULL CHECK (quantity > 0),
    line_total    NUMERIC(18,2) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_order_items_company_id ON order_items (company_id);
CREATE INDEX idx_order_items_order_id ON order_items (order_id);
