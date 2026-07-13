-- Melengkapi branch-level filtering (lihat journal_entries/invoices yang
-- sudah punya branch_id sejak 001_init.sql) ke accounts, supaya Chart of
-- Accounts juga bisa di-scope per branch seperti modul lain.
ALTER TABLE accounts ADD COLUMN branch_id UUID;
