-- Fase 3 (Manufacturing) -- Formula: BOM berbasis persentase/batch untuk
-- industri proses (makanan, kimia, farmasi). Ini potongan terakhir Fase 3.
--
-- Beda pokoknya dengan BOM diskrit yang sudah ada: di industri proses resep
-- tidak ditulis "berapa per unit produk", melainkan "berapa persen dari satu
-- batch". Satu batch punya ukuran tetap yang ditentukan kapasitas alat --
-- tangki 1.000 kg tidak bisa diisi 1.500 kg hanya karena permintaannya
-- segitu. Karena itu:
--
--   * bom_type = 'UNIT'  -> BOM diskrit, apa adanya seperti sebelum migrasi
--                           ini: tiap baris punya quantity_per_unit.
--   * bom_type = 'BATCH' -> Formula: BOM punya batch_size (hasil satu batch)
--                           dan tiap baris punya percentage dari batch itu.
--
-- Kolom lama TIDAK diubah artinya: seluruh BOM yang sudah ada otomatis
-- bertipe UNIT dan berperilaku persis seperti sebelumnya.
--
-- Tiga aturan formula ditegakkan di Go (internal/httpapi/boms.go &
-- work_orders.go), bukan di sini, karena semuanya lintas baris atau lintas
-- tabel -- CHECK constraint hanya melihat satu baris:
--   1. jumlah percentage seluruh baris >= 100% (neraca massa: tidak mungkin
--      keluar 1.000 kg produk dari input kurang dari 1.000 kg; kelebihan di
--      atas 100% adalah susut proses, dan itulah yang membuat yield < 100%);
--   2. quantity_planned sebuah work order dari BOM BATCH wajib kelipatan
--      bulat batch_size -- setengah batch bukan sesuatu yang bisa dijalankan
--      di lantai produksi, jadi angkanya DITOLAK, bukan diam-diam dibulatkan;
--   3. kebutuhan komponen di work_order_lines dihitung langsung dari
--      percentage x batch_size x batch_count, bukan lewat quantity_per_unit,
--      supaya persentase pecahan (mis. 33,3333%) tidak kehilangan ketelitian
--      dua kali.

ALTER TABLE bill_of_materials
    ADD COLUMN bom_type   VARCHAR(10) NOT NULL DEFAULT 'UNIT' CHECK (bom_type IN ('UNIT', 'BATCH')),
    -- Hasil satu batch, dalam satuan produk jadi. NULL untuk BOM UNIT.
    ADD COLUMN batch_size NUMERIC(15, 4),
    ADD CONSTRAINT bom_batch_size_matches_type CHECK (
        (bom_type = 'UNIT' AND batch_size IS NULL)
        OR (bom_type = 'BATCH' AND batch_size IS NOT NULL AND batch_size > 0)
    );

-- percentage: bagian baris ini dari satu batch, dalam persen. NULL untuk
-- baris BOM UNIT. quantity_per_unit tetap diisi untuk baris BATCH (=
-- percentage / 100) supaya pembaca lama -- dan kode apa pun yang sudah
-- terlanjur memakainya -- tidak melihat kolom kosong; yang dipakai untuk
-- menghitung kebutuhan work order adalah percentage-nya, lihat aturan 3 di
-- atas.
ALTER TABLE bom_lines
    ADD COLUMN percentage NUMERIC(7, 4) CHECK (percentage IS NULL OR percentage > 0);

-- Berapa batch yang dijalankan work order ini. NULL untuk work order dari
-- BOM UNIT -- di sana "batch" tidak punya arti.
ALTER TABLE work_orders
    ADD COLUMN batch_count INTEGER CHECK (batch_count IS NULL OR batch_count > 0);
