-- HR Service: pengajuan cuti (leave_requests) dan lembur (overtime_logs),
-- dua item Fase 4 (HRIS) di roadmap yang belum pernah dibangun.
--
-- Keduanya sengaja dibuat sebagai tabel transaksi ber-status dengan alur
-- persetujuan, bukan sekadar catatan bebas, karena keduanya BERPENGARUH KE UANG:
-- lembur yang disetujui menambah gross payroll, cuti tanpa gaji mengurangi
-- basic salary pro-rata. Nilai yang belum disetujui tidak boleh ikut terhitung,
-- jadi status bukan sekadar label tampilan.
--
-- employee_name disimpan sebagai snapshot (pola sama seperti payroll_details
-- dan project-service.timesheets): supaya rekap lama tetap terbaca apa adanya
-- kalau data karyawan kelak berubah atau di-nonaktifkan.

-- Pengajuan cuti. Rentang tanggal (bukan satu baris per hari) karena itu bentuk
-- pengajuan yang sebenarnya; jumlah hari kerja di dalam rentang dihitung ulang
-- per periode payroll lewat generate_series (lihat internal/httpapi/payroll.go),
-- bukan dibaca dari total_days, supaya cuti yang menyeberang bulan terpotong
-- benar di masing-masing periode.
CREATE TABLE leave_requests (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    branch_id        UUID,
    employee_id      UUID NOT NULL REFERENCES employees(id),
    employee_name    VARCHAR(200) NOT NULL,
    -- UNPAID dibedakan dari jenis lain karena hanya jenis inilah yang memotong
    -- gaji; ANNUAL/SICK/MATERNITY/OTHER tetap dibayar penuh.
    leave_type       VARCHAR(20) NOT NULL CHECK (leave_type IN ('ANNUAL', 'SICK', 'MATERNITY', 'UNPAID', 'OTHER')),
    start_date       DATE NOT NULL,
    end_date         DATE NOT NULL,
    -- Jumlah hari kerja (Sen-Jum) dalam rentang, dihitung saat create/update.
    -- Nilai tampilan & validasi kuota; BUKAN dasar perhitungan payroll.
    total_days       SMALLINT NOT NULL DEFAULT 0,
    reason           TEXT,
    status           VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT', 'SUBMITTED', 'APPROVED', 'REJECTED', 'CANCELLED')),
    rejection_reason TEXT,
    submitted_at     TIMESTAMPTZ,
    decided_at       TIMESTAMPTZ,
    decided_by       UUID,
    created_by       UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_date >= start_date)
);

CREATE INDEX idx_leave_requests_company_id ON leave_requests (company_id);
-- Dipakai baik oleh list per karyawan maupun oleh rekap payroll per periode,
-- yang selalu menyaring status = 'APPROVED' lebih dulu.
CREATE INDEX idx_leave_requests_employee_range ON leave_requests (employee_id, status, start_date, end_date);

-- Catatan lembur. Satu baris per karyawan per tanggal (UNIQUE) supaya jam
-- lembur di hari yang sama tidak bisa dicatat dua kali oleh dua orang berbeda
-- lalu dibayar dua kali.
CREATE TABLE overtime_logs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    branch_id        UUID,
    employee_id      UUID NOT NULL REFERENCES employees(id),
    employee_name    VARCHAR(200) NOT NULL,
    work_date        DATE NOT NULL,
    -- Batas 12 jam mengikuti batas lembur harian Pasal 26 PP 35/2021 (4 jam
    -- untuk hari kerja, lebih longgar untuk hari libur) -- dibatasi di sini
    -- sebagai pagar kewarasan input, bukan penegakan aturan penuh.
    hours            NUMERIC(5, 2) NOT NULL CHECK (hours > 0 AND hours <= 12),
    -- Lembur hari libur/istirahat mingguan dibayar dengan pengali berbeda,
    -- lihat calculateOvertimeAmount di internal/httpapi/overtime.go.
    is_holiday       BOOLEAN NOT NULL DEFAULT FALSE,
    -- Tarif per jam. Kalau tidak dikirim saat create, diturunkan dari gaji pokok
    -- karyawan (basic_salary / 173, Pasal 61 PP 35/2021).
    hourly_rate      NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (hourly_rate >= 0),
    -- Disimpan, bukan dihitung ulang saat dibaca: nilai yang sudah ikut payroll
    -- tidak boleh berubah kalau gaji karyawan kelak naik (alasan yang sama
    -- seperti payroll_details menyimpan snapshot gaji).
    amount           NUMERIC(18, 2) NOT NULL DEFAULT 0 CHECK (amount >= 0),
    description      TEXT,
    status           VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT', 'APPROVED', 'REJECTED')),
    rejection_reason TEXT,
    decided_at       TIMESTAMPTZ,
    decided_by       UUID,
    -- Terisi saat lembur ini ikut terhitung di sebuah payroll run. Sekali
    -- terisi, baris ini terkunci dari perubahan status (lihat overtime.go):
    -- mengubahnya setelah payroll diproses akan membuat gross payroll tidak
    -- bisa direkonsiliasi lagi dengan data lembur.
    payroll_run_id   UUID REFERENCES payroll_runs(id),
    created_by       UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (employee_id, work_date)
);

CREATE INDEX idx_overtime_logs_company_id ON overtime_logs (company_id);
CREATE INDEX idx_overtime_logs_employee_period ON overtime_logs (employee_id, status, work_date);

-- Payroll menyerap keduanya. Kolom baru diberi DEFAULT 0 supaya payroll run
-- yang sudah ada sebelum migrasi ini tetap valid (nilainya memang nol: waktu
-- itu belum ada data lembur/cuti sama sekali).
ALTER TABLE payroll_runs
    ADD COLUMN total_overtime NUMERIC(18, 2) NOT NULL DEFAULT 0;

ALTER TABLE payroll_details
    ADD COLUMN overtime_hours    NUMERIC(6, 2) NOT NULL DEFAULT 0,
    ADD COLUMN overtime_pay      NUMERIC(15, 2) NOT NULL DEFAULT 0,
    ADD COLUMN paid_leave_days   SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN unpaid_leave_days SMALLINT NOT NULL DEFAULT 0;
