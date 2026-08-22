-- KPI karyawan: item terakhir Fase 4 HRIS.
--
-- Tiga tabel: indikator (master), penilaian per karyawan per periode, dan
-- rincian nilai per indikator di dalam satu penilaian.
--
-- Keputusan yang menentukan bentuk tabelnya: rincian penilaian menyimpan
-- SALINAN nama, bobot, dan target indikator saat penilaian dibuat -- bukan
-- hanya foreign key ke kpi_indicators. Bobot dan target itu kebijakan yang
-- berubah tiap tahun; kalau hanya dirujuk, mengubah master indikator akan
-- diam-diam menulis ulang hasil penilaian periode yang sudah lewat. Ini
-- semangat yang sama dengan payroll_details yang menyimpan angka hasil
-- perhitungan, bukan menghitung ulang dari master gaji hari ini.

CREATE TABLE kpi_indicators (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id   UUID NOT NULL,
    code         VARCHAR(50) NOT NULL,
    name         VARCHAR(255) NOT NULL,
    description  TEXT,
    unit         VARCHAR(30) NOT NULL DEFAULT 'poin', -- %, unit, poin, rupiah, ...
    target_value NUMERIC(15,2) NOT NULL CHECK (target_value > 0),
    weight       NUMERIC(5,2) NOT NULL CHECK (weight > 0 AND weight <= 100), -- bobot dalam persen
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_by   UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_kpi_indicators_company ON kpi_indicators (company_id, is_active);

CREATE TABLE kpi_reviews (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL,
    branch_id     UUID,
    employee_id   UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    employee_name VARCHAR(255) NOT NULL, -- salinan, sama seperti leave_requests & payroll_details
    period        VARCHAR(7) NOT NULL,   -- YYYY-MM, format yang sama dengan payroll_runs
    status        VARCHAR(20) NOT NULL DEFAULT 'DRAFT', -- DRAFT | SUBMITTED | APPROVED | REJECTED
    -- total_score & rating disimpan (bukan dihitung saat dibaca) supaya hasil
    -- yang sudah disetujui tidak berubah kalau rumusnya nanti disesuaikan.
    total_score      NUMERIC(6,2) NOT NULL DEFAULT 0,
    rating           VARCHAR(30) NOT NULL DEFAULT '',
    notes            TEXT,
    rejection_reason TEXT,
    submitted_at     TIMESTAMPTZ,
    decided_at       TIMESTAMPTZ,
    decided_by       UUID,
    created_by       UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Satu penilaian per karyawan per periode: dua penilaian untuk periode yang
    -- sama tidak punya arti dan membuat rekap ganda.
    UNIQUE (employee_id, period)
);

CREATE INDEX idx_kpi_reviews_company_period ON kpi_reviews (company_id, period);

CREATE TABLE kpi_review_items (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id      UUID NOT NULL REFERENCES kpi_reviews(id) ON DELETE CASCADE,
    indicator_id   UUID NOT NULL REFERENCES kpi_indicators(id) ON DELETE RESTRICT,
    indicator_name VARCHAR(255) NOT NULL,  -- salinan saat penilaian dibuat
    unit           VARCHAR(30) NOT NULL,   -- salinan
    target_value   NUMERIC(15,2) NOT NULL, -- salinan
    weight         NUMERIC(5,2) NOT NULL,  -- salinan
    actual_value   NUMERIC(15,2) NOT NULL DEFAULT 0,
    -- achievement = actual/target*100 (dibatasi, lihat kpi.go), score =
    -- achievement * weight / 100. Keduanya disimpan supaya rincian di layar
    -- persis sama dengan angka yang menghasilkan total_score.
    achievement NUMERIC(6,2) NOT NULL DEFAULT 0,
    score       NUMERIC(6,2) NOT NULL DEFAULT 0,
    note        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (review_id, indicator_id)
);

CREATE INDEX idx_kpi_review_items_review ON kpi_review_items (review_id);
