-- Asset Service: pendataan aset perusahaan (assets) dan jadwal
-- maintenance-nya (maintenance_schedules). Mengikuti prinsip platform
-- (README root): setiap data transaksi membawa company_id/branch_id, tidak
-- ada FK fisik lintas database.
--
-- Beda dari modul lain (Warehouse/Production/QC), Asset TIDAK melibatkan
-- product master maupun stock -- aset fisik perusahaan (mesin, kendaraan,
-- komputer, dst), bukan barang dagangan/bahan baku. warehouse_id sengaja
-- opsional & informational saja (lokasi fisik aset ditaruh di gudang mana),
-- tanpa FK fisik ke warehouse-service dan TIDAK memicu apa pun di sana.
--
-- maintenance_schedules pakai FK fisik ke assets karena satu database yang
-- sama (bukan lintas service).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE assets (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        UUID NOT NULL,
    branch_id         UUID,
    warehouse_id      UUID, -- opsional, lokasi fisik aset; informational saja, lihat komentar di atas
    asset_code        VARCHAR(30) NOT NULL,
    name              VARCHAR(200) NOT NULL,
    category          VARCHAR(100),
    acquisition_date  DATE,
    acquisition_cost  NUMERIC(15, 2) NOT NULL DEFAULT 0,
    status            VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'MAINTENANCE', 'DISPOSED')),
    notes             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, asset_code)
);

CREATE INDEX idx_assets_company_id ON assets (company_id);

-- Jadwal maintenance per aset. status COMPLETED/CANCELLED bersifat final;
-- "overdue" (scheduled_date sudah lewat tapi masih SCHEDULED) dihitung on
-- the fly saat query/tampil, bukan status tersendiri yang butuh job
-- terjadwal untuk diperbarui.
CREATE TABLE maintenance_schedules (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    asset_id         UUID NOT NULL REFERENCES assets(id),
    maintenance_type VARCHAR(100) NOT NULL,
    scheduled_date   DATE NOT NULL,
    completed_date   DATE,
    status           VARCHAR(20) NOT NULL DEFAULT 'SCHEDULED' CHECK (status IN ('SCHEDULED', 'COMPLETED', 'CANCELLED')),
    notes            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_maintenance_schedules_company_id ON maintenance_schedules (company_id);
CREATE INDEX idx_maintenance_schedules_asset_id ON maintenance_schedules (asset_id);
CREATE INDEX idx_maintenance_schedules_scheduled_date ON maintenance_schedules (company_id, scheduled_date);
