# E-Commerce Service

Status: **Fase 3 — diimplementasikan (core module)** (2026-08-10). Modul bisnis baru ketiga di luar 9 modul Fase 2 awal (setelah CRM dan Ticketing) — pilihan terakhir dari opsi roadmap "New business module" (Ticketing dan E-Commerce sebelumnya ditawarkan bersamaan, Ticketing dipilih lebih dulu).

Role terkait: `ecommerce`.

## Lingkup

- **Orders** — checkout/pesanan online. `status` (`PENDING`/`PAID`/`SHIPPED`/`DELIVERED`/`CANCELLED`). Customer **standalone** (`customer_name`/`customer_email` teks bebas) — sengaja TIDAK di-link ke Account/Contact crm-service, pola yang sama dengan keputusan `tickets.requester_name`/`email` di ticketing-service (menghindari pola soft-reference lintas service baru).
- **Order Items** — baris keranjang per order, FK fisik biasa ke `orders`. **Katalog produk TIDAK diduplikasi di service ini** — `product_id` menunjuk langsung ke produk warehouse-service (tanpa FK, lintas service); frontend mengambil daftar produk lewat `GET /api/warehouse/products` untuk mengisi dropdown pemilihan produk saat checkout. `product_sku`/`product_name`/`unit_price` adalah SNAPSHOT pada saat order dibuat (harga jual tidak ada di warehouse-service, cuma `cost_price` — `unit_price` diinput manual sama seperti `sales_order_lines.unit_price` di sales-service), bukan live-lookup.

Status flow: **semua transisi lewat endpoint khusus** (`pay`/`ship`/`deliver`/`cancel`), tidak ada PUT generik untuk status — beda dari pola `tickets` (PUT bebas antar status terbuka + `/close` terminal), mengikuti pola `sales_orders` (`confirm`/`fulfill`/`invoice`) yang lebih cocok untuk dokumen transaksional satu-arah. `PUT /orders/{id}` cuma untuk field non-status (`customer_name`/`customer_email`/`shipping_address`/`notes`), dan hanya diizinkan selagi `PENDING`.

- `POST /orders/{id}/pay` — `PENDING` → `PAID`.
- `POST /orders/{id}/ship` — `PAID` → `SHIPPED`, **memicu stok keluar nyata di warehouse-service** (`POST /stock-movements/batch`, `movement_type=OUT`, `reference_type=ECOMMERCE_ORDER`) sebelum status lokal diubah — panggilan warehouse-service dulu, baru update status lokal setelah sukses, pola identik dengan `fulfillSalesOrder` di sales-service. Kalau warehouse-service gagal, order tetap `PAID` (502 Bad Gateway ke caller).
- `POST /orders/{id}/deliver` — `SHIPPED` → `DELIVERED` (terminal).
- `POST /orders/{id}/cancel` — hanya dari `PENDING` atau `PAID` (409 kalau sudah `SHIPPED`/`DELIVERED`/`CANCELLED`) — setelah `SHIPPED`, stok sudah keluar dari warehouse-service, jadi pembatalan butuh alur retur/stock-in balik yang belum ada di modul ini.

## Endpoint (lihat kode di `internal/httpapi/` untuk detail request/response)

```
GET    /orders?company_id=&branch_id=&status=
POST   /orders
GET    /orders/{id}                 (order header + items)
PUT    /orders/{id}
POST   /orders/{id}/pay
POST   /orders/{id}/ship            (body: warehouse_id)
POST   /orders/{id}/deliver
POST   /orders/{id}/cancel
```

Tidak ada endpoint `order_items` berdiri sendiri — baris keranjang cuma dibuat sebagai bagian dari `POST /orders` dan dibaca lewat `GET /orders/{id}` (embedded `items` array), meniru pola `fetchSalesOrderLines`/`salesOrderWithLines` di sales-service persis.

Semua endpoint diakses lewat api-gateway dengan prefix `/api/ecommerce/*` (lihat `backend/services/api-gateway/internal/gateway/gateway.go`).

## Menjalankan

```bash
# Buat database dulu kalau belum ada
psql -U platform -h localhost -c "CREATE DATABASE ecommerce_service;"

cd backend/modules/ecommerce-service
go run ./cmd/server        # migrasi jalan otomatis saat startup, listen di :8098
```

Butuh `warehouse-service` aktif (default `WAREHOUSE_SERVICE_URL=http://localhost:8089`) supaya `POST /orders/{id}/ship` bisa mencatat stok keluar sungguhan.

Frontend: `frontend/web/src/pages/ecommerce/OrdersPage.jsx`, route `/ecommerce/orders` (menu ter-seed di rbac-service migration `013_seed_ecommerce_menus.sql`).

## Belum ada / batasan yang disengaja

- **Belum ada fact table dw-service** untuk E-Commerce — tidak ada bukti kebutuhan BI/reporting saat modul ini dibangun.
- **Belum ada production-readiness** (Dockerfile, docker-compose service block, Kubernetes manifest, entri CI, env template staging/prod, target Prometheus) — sengaja ditunda ke sesi terpisah, sama pola dua-tahap seperti crm-service dan ticketing-service.
- **Tidak ada validasi live ke warehouse-service saat order dibuat** — `product_id`/`product_sku`/`product_name`/`unit_price` di setiap baris dipercaya apa adanya dari request (frontend sudah mengambilnya dari `GET /api/warehouse/products`), tidak ada panggilan HTTP balik untuk memverifikasi produk itu benar-benar ada atau harganya masuk akal. Sama seperti `sales_order_lines` di sales-service, ini trade-off yang disengaja untuk menghindari coupling ekstra saat create — kalau nanti terbukti perlu, validasi bisa ditambahkan di dalam transaksi `createOrder` (pola yang sama seperti `createTicket` memvalidasi `category_id` ke tabel lokalnya sendiri).
- Nomor order digenerate via `COUNT(*)+1` per company per periode (`nextSequence`, disalin dari sales-service/crm-service/ticketing-service) — cukup untuk dev/demo, bukan production-grade di bawah concurrency tinggi (batasan yang sama sudah didokumentasikan di `finance-service/README.md`, `crm-service/README.md`, dan `ticketing-service/README.md`).
- Customer TIDAK di-link ke CRM Account/Contact (keputusan sengaja, lihat "Lingkup" di atas) — kalau ternyata dibutuhkan nanti, pola soft-link (nullable UUID tanpa FK/validasi lintas service, karena beda database) bisa ditambahkan sebagai kolom opsional tanpa migrasi breaking.
- **Cancel setelah SHIPPED tidak didukung** — stok sudah keluar dari warehouse-service di titik itu, butuh alur retur (stock-in balik) yang belum dirancang.
