# Warehouse Service / Gudang

Status: **diimplementasikan (Fase 2)**, dengan test integrasi terhadap Postgres sungguhan.

Role terkait: `Warehouse`. Port `28089`, database `warehouse_service`, route gateway `/api/warehouse`.

Lingkup: produk, gudang, saldo dan mutasi stok, transfer antar gudang, dan stock opname. Semua pergerakan stok (manual, batch dari service lain, konfirmasi transfer, posting opname) lewat satu fungsi yang juga menegakkan aturan stok tidak boleh negatif.

## Endpoint

| Method | Path | Fungsi |
|---|---|---|
| GET/POST | `/products` | Daftar / buat produk |
| PUT | `/products/{id}` | Ubah produk |
| GET/POST | `/warehouses` | Daftar / buat gudang |
| PUT | `/warehouses/{id}` | Ubah gudang |
| GET | `/stock` | Saldo stok per gudang |
| GET/POST | `/stock-movements` | Riwayat / catat mutasi manual (IN atau OUT) |
| POST | `/stock-movements/batch` | Mutasi massal dari service lain (sales, e-commerce, purchasing, production) |
| GET/POST | `/stock-transfers` | Daftar / buat transfer antar gudang |
| GET | `/stock-transfers/{id}` | Detail |
| POST | `/stock-transfers/{id}/confirm` | Konfirmasi: stok pindah |
| GET/POST | `/stock-opnames` | Daftar / buat stock opname |
| GET | `/stock-opnames/{id}` | Detail |
| POST | `/stock-opnames/{id}/post` | Post: selisih dicatat sebagai penyesuaian |

Di gateway, path ini diawali `/api/warehouse` dan izinnya dicek per menu (lihat `backend/services/api-gateway/internal/authz/policy.go`).

## Event Kafka

warehouse.product.created/updated, warehouse.warehouse.created/updated, warehouse.stock.moved, warehouse.transfer.created/confirmed, warehouse.opname.created/posted.

## Memanggil service lain

tidak ada (dipanggil oleh sales, purchasing, e-commerce, production).

## Aturan stok tidak boleh negatif (2026-10-21)

Mutasi `OUT` hanya berhasil kalau saldo gudang+produk mencukupi; jika tidak, respons `409` dengan pesan yang menyebut jumlah diminta dan tersedia, dan kode `INSUFFICIENT_STOCK`. Mutasi `IN` tidak pernah ditolak.

- **Satu titik penegakan:** `applyStockMovement` (`internal/httpapi/stock_movements.go`). Semua alur lewat sana: mutasi manual, batch dari service lain (sales fulfill, e-commerce ship, production complete), konfirmasi transfer, dan posting opname. Pengurangan ditulis sebagai `UPDATE ... WHERE quantity >= diminta`, jadi dua OUT bersamaan atas saldo yang sama tidak bisa sama-sama lolos (diuji dengan 8 permintaan paralel).
- **Semua-atau-tidak-sama-sekali:** satu baris batch yang kurang membatalkan seluruh batch dan tidak meninggalkan baris ledger. Konfirmasi transfer yang gudang asalnya kurang tetap berstatus DRAFT.
- **Tidak ada `CHECK (quantity >= 0)` di database, sengaja:** constraint itu juga akan menolak IN pada baris yang sudah negatif dari masa sebelum aturan ini (mis. -320 di data dev), padahal barang masuk adalah satu-satunya cara saldo itu pulih. Saldo lama yang negatif dibiarkan; OUT atasnya ditolak sampai pulih.
- **Efek ke service lain:** sales/e-commerce/production meneruskan penolakan ini sebagai `409` dengan pesan stok (bukan `502` berisi JSON mentah), dan status dokumennya tidak berubah. Produk yang tidak cocok namanya (SO memakai `product_name`) kini dibuat sebagai produk baru bersaldo nol dan ditolak -- dulu itu diam-diam menghasilkan saldo negatif.
- **Opname:** penyesuaian turun dihitung dari snapshot saat opname dibuat. Kalau stok keluar sesudahnya dan penyesuaian melampaui saldo saat ini, posting ditolak (409) alih-alih membuat saldo negatif; buat opname baru dari saldo terkini.
