-- Menambah 'ECOMMERCE_ORDER' ke daftar reference_type yang valid di
-- stock_movements, supaya ecommerce-service (backend/modules/ecommerce-service)
-- bisa mencatat stok keluar lewat POST /stock-movements/batch saat order
-- SHIPPED (lihat validReferenceTypes di internal/httpapi/stock_movements.go,
-- yang sudah lebih dulu diupdate -- pola identik dengan migrasi 002 untuk
-- WORK_ORDER).

ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_reference_type_check;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_reference_type_check
    CHECK (reference_type IN ('PURCHASE_ORDER', 'SALES_ORDER', 'TRANSFER', 'OPNAME', 'MANUAL', 'WORK_ORDER', 'ECOMMERCE_ORDER'));
