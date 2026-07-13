-- Melengkapi branch-level filtering (lihat stock_movements yang sudah punya
-- branch_id sejak 001_init.sql) ke stock_transfers dan stock_opnames.
-- stock_balances SENGAJA tidak disentuh -- itu saldo teragregasi per
-- warehouse+product (bukan record transaksional per-branch), dan warehouse
-- sendiri company-wide sesuai keputusan produk (lihat StockPage.jsx).
ALTER TABLE stock_transfers ADD COLUMN branch_id UUID;
ALTER TABLE stock_opnames ADD COLUMN branch_id UUID;
