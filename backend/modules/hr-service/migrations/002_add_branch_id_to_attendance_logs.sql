-- Melengkapi branch-level filtering (lihat employees/payroll_runs yang sudah
-- punya branch_id sejak 001_init.sql) ke attendance_logs.
ALTER TABLE attendance_logs ADD COLUMN branch_id UUID;
