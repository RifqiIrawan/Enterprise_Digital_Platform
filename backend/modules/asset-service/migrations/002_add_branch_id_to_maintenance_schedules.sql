-- Melengkapi branch-level filtering (lihat assets yang sudah punya branch_id
-- sejak 001_init.sql) ke maintenance_schedules.
ALTER TABLE maintenance_schedules ADD COLUMN branch_id UUID;
