-- Menambah 'WORK_ORDER' ke daftar reference_type yang valid di
-- stock_movements, supaya production-service (backend/modules/production-service)
-- bisa mencatat konsumsi komponen & hasil produksi lewat POST
-- /stock-movements/batch saat work order COMPLETED (lihat validReferenceTypes
-- di internal/httpapi/stock_movements.go, yang sudah lebih dulu diupdate).

ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_reference_type_check;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_reference_type_check
    CHECK (reference_type IN ('PURCHASE_ORDER', 'SALES_ORDER', 'TRANSFER', 'OPNAME', 'MANUAL', 'WORK_ORDER'));
