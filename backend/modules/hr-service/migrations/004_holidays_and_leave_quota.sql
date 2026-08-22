-- Master kalender hari libur & kuota cuti tahunan (dua lubang yang ditinggalkan
-- 003_leave_and_overtime.sql).
--
-- Sebelum ini, "hari kerja" di seluruh hr-service berarti Senin-Jumat polos:
-- tanggal merah tetap dihitung sebagai hari kerja saat pro-rata payroll, ikut
-- memotong jatah cuti karyawan, dan `is_holiday` di lembur diisi manual lewat
-- checkbox sehingga bergantung pada ketelitian yang menginput.
--
-- Kalender disimpan PER COMPANY, bukan global: tanggal merah nasional memang
-- sama untuk semua, tapi cuti bersama dan libur khusus perusahaan tidak. Kolom
-- is_national membedakan keduanya untuk keperluan tampilan/laporan; keduanya
-- sama-sama berarti "bukan hari kerja".
CREATE TABLE holidays (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id   UUID NOT NULL,
    holiday_date DATE NOT NULL,
    name         VARCHAR(255) NOT NULL,
    is_national  BOOLEAN NOT NULL DEFAULT TRUE, -- FALSE = libur khusus perusahaan / cuti bersama internal
    created_by   UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, holiday_date)
);

CREATE INDEX idx_holidays_company_date ON holidays (company_id, holiday_date);

-- Kuota cuti tahunan per karyawan. Default 12 hari mengikuti batas minimum UU
-- Ketenagakerjaan; carried_over menampung sisa tahun sebelumnya yang dibawa
-- (kebijakan tiap perusahaan berbeda, jadi angkanya di-input, bukan dihitung
-- otomatis oleh sistem).
--
-- Yang TIDAK disimpan di sini: jumlah hari yang sudah terpakai. Itu selalu
-- dihitung ulang dari leave_requests berstatus APPROVED, supaya tidak ada dua
-- sumber kebenaran yang bisa lepas sinkron saat cuti dibatalkan atau ditolak.
CREATE TABLE leave_quotas (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id  UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    year         INT NOT NULL,
    total_days   INT NOT NULL DEFAULT 12 CHECK (total_days >= 0),
    carried_over INT NOT NULL DEFAULT 0 CHECK (carried_over >= 0),
    note         TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (employee_id, year)
);

CREATE INDEX idx_leave_quotas_employee_year ON leave_quotas (employee_id, year);
