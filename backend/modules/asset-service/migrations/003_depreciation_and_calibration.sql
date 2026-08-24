-- Fase 5 (Asset) -- dua hal yang tersisa dari roadmap modul ini: penyusutan
-- (depreciation) dan kalibrasi. Pendataan aset & maintenance sudah ada sejak
-- Fase 2.
--
-- Penyusutan mengikuti pola payroll di hr-service: sebuah RUN per periode yang
-- menghitung dulu (DRAFT), baru diposting ke GL lewat finance-service. Yang
-- membuatnya bukan sekadar tabel angka: begitu diposting, dia menyentuh buku
-- besar, jadi urutannya harus terjaga dan hasilnya tidak boleh bisa dihitung
-- dua kali untuk periode yang sama.

ALTER TABLE assets
    ADD COLUMN salvage_value            NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (salvage_value >= 0),
    -- NULL = aset ini memang tidak disusutkan (tanah, aset yang sudah habis
    -- disusutkan sebelum sistem ini dipakai, barang di bawah batas kapitalisasi).
    ADD COLUMN useful_life_months       INTEGER CHECK (useful_life_months > 0),
    ADD COLUMN depreciation_method      VARCHAR(20) NOT NULL DEFAULT 'STRAIGHT_LINE'
                                        CHECK (depreciation_method IN ('STRAIGHT_LINE', 'DECLINING_BALANCE')),
    -- Kapan penyusutan mulai dihitung; biasanya sama dengan acquisition_date,
    -- tapi tidak selalu (aset yang baru dipakai beberapa bulan setelah dibeli).
    ADD COLUMN depreciation_start_date  DATE,
    -- Total penyusutan yang SUDAH diposting. Sengaja disimpan (bukan selalu
    -- dijumlahkan dari depreciation_entries): setiap perhitungan run butuh
    -- nilai buku tiap aset, dan itu pertanyaan yang muncul di setiap layar
    -- aset juga. Yang menaikkannya hanya posting run -- lihat postDepreciationRun.
    ADD COLUMN accumulated_depreciation NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (accumulated_depreciation >= 0);

-- Satu run = satu periode (bulan) untuk satu company. DRAFT boleh dihapus dan
-- dihitung ulang; POSTED tidak bisa disentuh lagi.
--
-- SENGAJA tidak ber-branch, berbeda dari tabel transaksi lain di platform ini:
-- penyusutan adalah penutupan buku tingkat company, dan run yang dipecah per
-- cabang membuat satu periode punya beberapa run tanpa jaminan seluruh aset
-- tersapu -- aset tanpa branch_id akan terlewat atau terhitung berkali-kali.
-- Pelaporan per cabang tetap bisa: entry-nya menunjuk aset, dan aset yang
-- membawa branch_id.
CREATE TABLE depreciation_runs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    period           CHAR(7) NOT NULL, -- 'YYYY-MM'
    status           VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'POSTED')),
    asset_count      INTEGER NOT NULL DEFAULT 0,
    total_amount     NUMERIC(15, 2) NOT NULL DEFAULT 0,
    -- ID journal entry di finance-service (database lain, jadi tanpa FK fisik).
    journal_entry_id UUID,
    posted_at        TIMESTAMPTZ,
    notes            TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Penyusutan sebuah periode tidak boleh dihitung dua kali. Ini di database,
    -- bukan cuma di handler: dua orang yang menekan "Hitung Penyusutan" untuk
    -- bulan yang sama akan lolos pemeriksaan SELECT keduanya.
    UNIQUE (company_id, period),
    CHECK ((status = 'DRAFT'  AND journal_entry_id IS NULL AND posted_at IS NULL)
        OR (status = 'POSTED' AND journal_entry_id IS NOT NULL AND posted_at IS NOT NULL))
);

CREATE INDEX idx_depreciation_runs_company_id ON depreciation_runs (company_id, period DESC);

-- Satu baris per aset yang disusutkan pada periode itu. book_value_before/after
-- disimpan supaya angka periode lama tetap bisa dibaca apa adanya walau master
-- asetnya (umur manfaat, nilai residu) disesuaikan kemudian.
CREATE TABLE depreciation_entries (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id            UUID NOT NULL REFERENCES depreciation_runs(id) ON DELETE CASCADE,
    asset_id          UUID NOT NULL REFERENCES assets(id),
    method            VARCHAR(20) NOT NULL,
    amount            NUMERIC(15, 2) NOT NULL CHECK (amount > 0),
    book_value_before NUMERIC(15, 2) NOT NULL,
    book_value_after  NUMERIC(15, 2) NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (run_id, asset_id)
);

CREATE INDEX idx_depreciation_entries_run_id ON depreciation_entries (run_id);
CREATE INDEX idx_depreciation_entries_asset_id ON depreciation_entries (asset_id);

-- Kalibrasi: kewajiban berulang, bukan kejadian sekali. Karena itu
-- completeCalibration otomatis menjadwalkan kalibrasi berikutnya kalau
-- interval_months diisi -- alat ukur yang kalibrasinya kedaluwarsa adalah
-- alat yang hasil ukurnya tidak bisa dipakai, dan mengandalkan orang untuk
-- ingat menjadwalkan ulang adalah cara paling mudah kehilangan jejaknya.
--
-- "Overdue" (scheduled_date sudah lewat tapi masih SCHEDULED) dihitung saat
-- ditampilkan, bukan status tersendiri -- sama seperti maintenance_schedules.
CREATE TABLE calibrations (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id         UUID NOT NULL,
    branch_id          UUID,
    asset_id           UUID NOT NULL REFERENCES assets(id),
    scheduled_date     DATE NOT NULL,
    performed_date     DATE,
    -- Jarak ke kalibrasi berikutnya. NULL = kalibrasi sekali saja.
    interval_months    INTEGER CHECK (interval_months > 0),
    next_due_date      DATE,
    result             VARCHAR(20) CHECK (result IN ('PASS', 'FAIL', 'ADJUSTED')),
    certificate_number VARCHAR(100),
    performed_by       VARCHAR(200),
    status             VARCHAR(20) NOT NULL DEFAULT 'SCHEDULED' CHECK (status IN ('SCHEDULED', 'COMPLETED', 'CANCELLED')),
    notes              TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Hasil kalibrasi hanya ada setelah dikerjakan, dan begitu selesai wajib ada.
    CHECK ((status <> 'COMPLETED' AND result IS NULL AND performed_date IS NULL)
        OR (status =  'COMPLETED' AND result IS NOT NULL AND performed_date IS NOT NULL))
);

CREATE INDEX idx_calibrations_company_id ON calibrations (company_id);
CREATE INDEX idx_calibrations_asset_id ON calibrations (asset_id);
CREATE INDEX idx_calibrations_scheduled_date ON calibrations (company_id, scheduled_date);
