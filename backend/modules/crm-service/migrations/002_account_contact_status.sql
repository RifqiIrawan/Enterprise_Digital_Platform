-- Status aktif/nonaktif untuk Account & Contact.
--
-- Sebelum ini keduanya adalah SATU-SATUNYA master data di seluruh platform
-- yang tidak bisa dihapus DAN tidak punya cara dinonaktifkan: sekali salah
-- input (atau sekali sebuah perusahaan berhenti jadi pelanggan), barisnya
-- tinggal menumpuk di daftar tanpa ada jalan keluar lewat UI. 14 master lain
-- (customers, suppliers, products, vehicles, devices, ...) semuanya sudah
-- punya status/is_active sejak awal.
--
-- Dipilih menonaktifkan, bukan menghapus, karena account/contact selalu sudah
-- terlanjur dirujuk opportunity dan activity -- menghapusnya akan meninggalkan
-- riwayat penjualan yang menunjuk ke nama yang hilang.
--
-- DEFAULT 'ACTIVE' membuat seluruh baris yang sudah ada tetap seperti semula.

ALTER TABLE accounts ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE', 'INACTIVE'));

ALTER TABLE contacts ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE', 'INACTIVE'));

CREATE INDEX idx_accounts_company_status ON accounts (company_id, status);
CREATE INDEX idx_contacts_company_status ON contacts (company_id, status);
