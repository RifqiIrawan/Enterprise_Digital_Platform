-- Fase 3 (Manufacturing) -- lapisan MES di atas BOM & Work Order yang sudah
-- ada: mesin, shift, catatan eksekusi produksi per mesin per shift
-- (production_runs), downtime, dan bahan perhitungan OEE.
--
-- Pembagian tanggung jawab yang perlu diingat: Work Order adalah RENCANA
-- (berapa yang mau dibuat, dari BOM mana, ke gudang mana) dan satu-satunya
-- yang menyentuh stok saat COMPLETED. Production run adalah CATATAN
-- PELAKSANAAN di lantai produksi -- tidak menyentuh stok sama sekali, tapi
-- dialah yang tahu berapa yang benar-benar keluar bagus, berapa yang reject,
-- dan berapa lama mesinnya berhenti. Lihat completeWorkOrder di
-- internal/httpapi/work_orders.go soal bagaimana keduanya dijahit: begitu
-- sebuah WO punya production run, angka hasil produksinya diambil dari run,
-- bukan dari yang diketik operator.

CREATE TABLE machines (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id               UUID NOT NULL,
    branch_id                UUID,
    code                     VARCHAR(30) NOT NULL,
    name                     VARCHAR(200) NOT NULL,
    machine_type             VARCHAR(50) NOT NULL DEFAULT 'GENERAL',
    location                 VARCHAR(200),
    -- Menit yang secara teoritis dibutuhkan mesin ini untuk 1 unit produk
    -- pada kecepatan rancangannya. Ini penyebut faktor Performance di OEE;
    -- tanpa angka ini OEE tidak bisa dihitung sama sekali, jadi wajib.
    ideal_cycle_time_minutes NUMERIC(10, 4) NOT NULL CHECK (ideal_cycle_time_minutes > 0),
    status                   VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'MAINTENANCE', 'INACTIVE')),
    notes                    TEXT NOT NULL DEFAULT '',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_machines_company_id ON machines (company_id);
CREATE INDEX idx_machines_status ON machines (company_id, status);

-- Shift dipakai sebagai SUMBER WAKTU RENCANA sebuah production run: durasi
-- shift dikurangi istirahat = planned_minutes, penyebut faktor Availability.
-- Shift yang melewati tengah malam (mis. 22:00-06:00) valid; durasinya
-- dihitung di Go, lihat shiftPlannedMinutes di internal/httpapi/shifts.go.
CREATE TABLE shifts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL,
    code          VARCHAR(30) NOT NULL,
    name          VARCHAR(200) NOT NULL,
    start_time    TIME NOT NULL,
    end_time      TIME NOT NULL,
    break_minutes INTEGER NOT NULL DEFAULT 0 CHECK (break_minutes >= 0),
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, code),
    CHECK (start_time <> end_time)
);

CREATE INDEX idx_shifts_company_id ON shifts (company_id);

-- Satu baris = satu mesin, satu shift, satu tanggal, mengerjakan satu work
-- order. planned_minutes di-SNAPSHOT dari shift saat run dibuat supaya jam
-- shift yang diubah bulan depan tidak diam-diam mengubah OEE bulan lalu --
-- alasan yang sama seperti work_order_lines men-snapshot bom_lines.
CREATE TABLE production_runs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    branch_id        UUID,
    run_number       VARCHAR(30) NOT NULL,
    work_order_id    UUID NOT NULL REFERENCES work_orders(id),
    machine_id       UUID NOT NULL REFERENCES machines(id),
    shift_id         UUID NOT NULL REFERENCES shifts(id),
    run_date         DATE NOT NULL,
    planned_minutes  INTEGER NOT NULL CHECK (planned_minutes > 0),
    quantity_good    NUMERIC(15, 2),
    quantity_reject  NUMERIC(15, 2),
    status           VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'CLOSED')),
    notes            TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, run_number),
    -- Angka hasil hanya ada setelah run ditutup, dan begitu ditutup keduanya
    -- wajib ada. Tanpa CHECK ini, run OPEN dengan quantity_good terisi akan
    -- ikut terhitung di OEE padahal belum selesai.
    CHECK ((status = 'OPEN'  AND quantity_good IS NULL AND quantity_reject IS NULL)
        OR (status = 'CLOSED' AND quantity_good IS NOT NULL AND quantity_reject IS NOT NULL))
);

-- Sebuah mesin hanya mengerjakan satu hal pada satu waktu. Ini dijaga di
-- database, bukan hanya di handler: dua operator yang menekan "Mulai Run"
-- bersamaan pada mesin yang sama akan lolos pemeriksaan SELECT keduanya.
CREATE UNIQUE INDEX idx_production_runs_one_open_per_machine
    ON production_runs (machine_id) WHERE status = 'OPEN';

CREATE INDEX idx_production_runs_company_id ON production_runs (company_id);
CREATE INDEX idx_production_runs_work_order_id ON production_runs (work_order_id);
CREATE INDEX idx_production_runs_machine_date ON production_runs (company_id, machine_id, run_date);

-- Downtime dicatat per run dengan alasan yang terbatas pilihannya supaya bisa
-- dijumlahkan lintas run (Pareto penyebab berhenti). PLANNED_STOP tetap
-- memotong Availability: OEE memang mengukur terhadap waktu yang sudah
-- dijadwalkan untuk berproduksi, dan istirahat sudah dikeluarkan lebih dulu
-- lewat break_minutes shift.
CREATE TABLE downtime_logs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    production_run_id UUID NOT NULL REFERENCES production_runs(id) ON DELETE CASCADE,
    reason_code       VARCHAR(30) NOT NULL CHECK (reason_code IN ('BREAKDOWN', 'SETUP', 'MATERIAL_SHORTAGE', 'NO_OPERATOR', 'ADJUSTMENT', 'PLANNED_STOP', 'OTHER')),
    minutes           INTEGER NOT NULL CHECK (minutes > 0),
    notes             TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_downtime_logs_production_run_id ON downtime_logs (production_run_id);
