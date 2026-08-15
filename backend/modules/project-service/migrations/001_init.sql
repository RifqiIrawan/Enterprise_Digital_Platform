-- Project Service: proyek, tugas, dan timesheet. Mengikuti prinsip platform
-- (README root): setiap data transaksi membawa company_id/branch_id, tidak ada
-- FK fisik lintas database.
--
-- Tiga entitas yang saling terkait DI DALAM service ini (FK fisik boleh,
-- semuanya satu database): projects <- tasks <- timesheets. timesheets.task_id
-- NULLABLE karena tidak semua jam kerja bisa dipetakan ke satu tugas tertentu
-- (rapat proyek, koordinasi, perjalanan) -- yang wajib cuma project_id.
--
-- Karyawan TIDAK disimpan di sini. employee_id di ketiga tabel adalah UUID
-- milik hr-service tanpa FK (beda database/service); nama karyawannya
-- di-SNAPSHOT (manager_name/assignee_name/employee_name) saat penugasan, pola
-- yang sama seperti fleet-service men-snapshot nama/alamat penerima dari
-- ecommerce-service dan ecommerce-service men-snapshot product_name dari
-- warehouse-service. Snapshot dipakai supaya daftar proyek/tugas tetap
-- terbaca tanpa memanggil hr-service di setiap request list, dan supaya
-- riwayat tetap masuk akal kalau karyawannya kelak berganti nama.
--
-- Alur status proyek lewat endpoint transisi khusus (activate/hold/complete/
-- cancel), BUKAN PUT generik -- pola yang sama seperti delivery_orders di
-- fleet-service dan orders di ecommerce-service. Tugas sengaja BEDA: status
-- tugas ikut PUT biasa (pola tickets di ticketing-service), karena tugas
-- berpindah status berkali-kali sehari dan tidak punya efek samping lintas
-- entitas -- satu-satunya otomatisasi adalah completed_at yang diisi/dikosongi
-- mengikuti status DONE.
--
-- Uang: budget_amount diinput saat perencanaan, actual_cost TIDAK diinput
-- manusia sama sekali -- dia hanya bertambah saat timesheet APPROVED
-- benar-benar diposting ke GL finance-service (lihat postProjectCost di
-- internal/httpapi/timesheets.go). Jadi actual_cost selalu bisa direkonsiliasi
-- dengan jurnal yang ada di finance-service, bukan angka yang berdiri sendiri.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE projects (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          UUID NOT NULL,
    branch_id           UUID,
    project_code        VARCHAR(30) NOT NULL,
    name                VARCHAR(200) NOT NULL,
    description         TEXT,
    customer_name       VARCHAR(200),
    -- Manajer proyek: karyawan hr-service, tanpa FK (lintas service).
    manager_employee_id UUID,
    manager_name        VARCHAR(200),
    start_date          DATE NOT NULL DEFAULT CURRENT_DATE,
    end_date            DATE,
    status              VARCHAR(20) NOT NULL DEFAULT 'PLANNING'
        CHECK (status IN ('PLANNING', 'ACTIVE', 'ON_HOLD', 'COMPLETED', 'CANCELLED')),
    budget_amount       NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (budget_amount >= 0),
    -- Hanya bertambah lewat posting timesheet ke GL, tidak pernah diinput manual.
    actual_cost         NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (actual_cost >= 0),
    completed_at        TIMESTAMPTZ,
    cancelled_at        TIMESTAMPTZ,
    notes               TEXT,
    created_by_user_id  UUID, -- dari header X-User-Id, tanpa FK (lintas service)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, project_code)
);

CREATE INDEX idx_projects_company_id ON projects (company_id);
CREATE INDEX idx_projects_status ON projects (company_id, status);

CREATE TABLE tasks (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id           UUID NOT NULL,
    branch_id            UUID,
    project_id           UUID NOT NULL REFERENCES projects(id),
    task_number          VARCHAR(30) NOT NULL,
    title                VARCHAR(200) NOT NULL,
    description          TEXT,
    -- Penanggung jawab: karyawan hr-service, tanpa FK. NULL = belum ditugaskan.
    assignee_employee_id UUID,
    assignee_name        VARCHAR(200),
    status               VARCHAR(20) NOT NULL DEFAULT 'TODO'
        CHECK (status IN ('TODO', 'IN_PROGRESS', 'DONE', 'CANCELLED')),
    priority             VARCHAR(10) NOT NULL DEFAULT 'MEDIUM'
        CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH')),
    due_date             DATE,
    estimated_hours      NUMERIC(8,2) NOT NULL DEFAULT 0 CHECK (estimated_hours >= 0),
    completed_at         TIMESTAMPTZ,
    created_by_user_id   UUID,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, task_number)
);

CREATE INDEX idx_tasks_company_id ON tasks (company_id);
CREATE INDEX idx_tasks_project_id ON tasks (project_id);
CREATE INDEX idx_tasks_status ON tasks (company_id, status);

CREATE TABLE timesheets (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          UUID NOT NULL,
    branch_id           UUID,
    project_id          UUID NOT NULL REFERENCES projects(id),
    -- NULL = jam kerja proyek yang tidak terikat satu tugas (rapat, koordinasi).
    task_id             UUID REFERENCES tasks(id),
    employee_id         UUID NOT NULL,
    employee_name       VARCHAR(200) NOT NULL,
    work_date           DATE NOT NULL DEFAULT CURRENT_DATE,
    hours               NUMERIC(6,2) NOT NULL CHECK (hours > 0 AND hours <= 24),
    -- Tarif per jam. Kalau tidak diisi saat create, diturunkan dari gaji pokok
    -- karyawan di hr-service (basic_salary / 173, lihat internal/httpapi/timesheets.go).
    hourly_rate         NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (hourly_rate >= 0),
    -- Disimpan (bukan dihitung on the fly) supaya nilai yang sudah diposting ke
    -- GL tidak berubah kalau tarifnya kelak diperbarui -- alasan yang sama
    -- seperti order_items menyimpan harga saat checkout.
    amount              NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (amount >= 0),
    description         TEXT,
    status              VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT', 'APPROVED', 'POSTED', 'REJECTED')),
    approved_at         TIMESTAMPTZ,
    posted_at           TIMESTAMPTZ,
    -- Journal entry finance-service hasil posting, tanpa FK (lintas service).
    journal_entry_id    UUID,
    created_by_user_id  UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_timesheets_company_id ON timesheets (company_id);
CREATE INDEX idx_timesheets_project_id ON timesheets (project_id);
CREATE INDEX idx_timesheets_status ON timesheets (company_id, status);
CREATE INDEX idx_timesheets_employee_id ON timesheets (employee_id);
