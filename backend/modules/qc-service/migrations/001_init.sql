-- QC Service: Standar Mutu (quality_standards) dan Inspeksi Kualitas
-- (quality_inspections). Mengikuti prinsip platform (README root): setiap
-- data transaksi membawa company_id/branch_id, tidak ada FK fisik lintas
-- database.
--
-- product_id menunjuk ke tabel `products` milik warehouse-service tanpa FK
-- fisik (beda database) -- qc-service percaya product_id yang dikirim
-- frontend (yang ambil daftar produk langsung dari GET /api/warehouse/products).
--
-- Berbeda dari modul Purchasing/Sales/Production, QC sengaja dibuat lebih
-- ringan: inspeksi TIDAK memicu mutasi stok otomatis di warehouse-service
-- (keputusan produk: hasil FAIL dikoreksi manual lewat Stock Opname/manual
-- movement di Warehouse, bukan otomatis). Karena itu tidak ada
-- internal/warehouseclient maupun status DRAFT/POSTED di sini -- satu
-- inspeksi = satu catatan hasil yang sudah final saat dibuat.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE quality_standards (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL,
    branch_id     UUID,
    standard_code VARCHAR(30) NOT NULL,
    name          VARCHAR(200) NOT NULL,
    product_id    UUID NOT NULL,
    criteria      TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, standard_code)
);

CREATE INDEX idx_quality_standards_company_id ON quality_standards (company_id);

-- reference_type/reference_id menunjuk ke entitas yang diinspeksi
-- (PURCHASE_ORDER, WORK_ORDER, MANUAL), tanpa FK fisik karena beda
-- database; reference_number disimpan sebagai teks bebas (mis. nomor PO)
-- supaya list inspeksi bisa langsung tampil tanpa perlu memanggil service
-- lain untuk sekadar menampilkan nomor referensi.
CREATE TABLE quality_inspections (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        UUID NOT NULL,
    branch_id         UUID,
    inspection_number VARCHAR(30) NOT NULL,
    standard_id       UUID NOT NULL REFERENCES quality_standards(id),
    product_id        UUID NOT NULL,
    reference_type    VARCHAR(20) NOT NULL DEFAULT 'MANUAL' CHECK (reference_type IN ('PURCHASE_ORDER', 'WORK_ORDER', 'MANUAL')),
    reference_id      UUID,
    reference_number  VARCHAR(30),
    inspected_quantity NUMERIC(15, 2) NOT NULL CHECK (inspected_quantity > 0),
    passed_quantity   NUMERIC(15, 2) NOT NULL DEFAULT 0,
    failed_quantity   NUMERIC(15, 2) NOT NULL DEFAULT 0,
    result            VARCHAR(10) NOT NULL CHECK (result IN ('PASS', 'FAIL', 'PARTIAL')),
    inspection_date   DATE NOT NULL,
    notes             TEXT,
    inspected_by      UUID,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, inspection_number)
);

CREATE INDEX idx_quality_inspections_company_id ON quality_inspections (company_id);
CREATE INDEX idx_quality_inspections_standard_id ON quality_inspections (standard_id);
