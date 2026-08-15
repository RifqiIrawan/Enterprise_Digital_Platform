# fleet-service (Fleet & Delivery)

Modul ke-20. Armada kendaraan, pengemudi, dan surat jalan (delivery order),
dengan integrasi dua arah ke `ecommerce-service`.

Port `8099`, database `fleet_service`, route gateway `/api/fleet`.

Status: **diimplementasikan, termasuk production-readiness** (modul inti
2026-08-12, production-readiness 2026-08-13/15) — Dockerfile, blok
docker-compose, manifest K8s base + overlay dev/staging/prod, env template
staging/production, entri CI, dan target scrape Prometheus semuanya sudah ada.
Image-nya sudah benar-benar di-build (`docker compose build fleet-service`) dan
container-nya di-smoke test terhadap Postgres native (2026-08-15): `/health`
200, `/metrics` melayani metrik Prometheus, migrasi jalan sendiri saat startup
(3 tabel + `schema_migrations`), dan `GET /vehicles` mengembalikan data demo
nyata dari dalam container.

## Lingkup

| Entitas | Keterangan |
|---|---|
| `vehicles` | Master armada. Kode diisi manual (master data, pola `customers.customer_code`). Status `AVAILABLE`/`IN_USE`/`MAINTENANCE`. |
| `drivers` | Master pengemudi. Status `AVAILABLE`/`ON_DELIVERY`/`INACTIVE`. |
| `delivery_orders` | Surat jalan. Nomor auto-generate (`DLV-202608-0001`). Status `PENDING` → `DISPATCHED` → `DELIVERED`, atau `CANCELLED` dari dua status pertama. |

## Yang membedakan modul ini

**Status kendaraan & pengemudi digerakkan oleh lifecycle surat jalan, bukan
diinput manual.** `dispatch` menandai kendaraan `IN_USE` + pengemudi
`ON_DELIVERY`; `deliver`/`cancel` mengembalikan keduanya ke `AVAILABLE` —
semuanya dalam SATU transaksi bersama perubahan status surat jalannya, dengan
`SELECT ... FOR UPDATE` pada ketiga baris. Tanpa penguncian itu, dua surat
jalan yang di-dispatch bersamaan bisa sama-sama lolos cek "AVAILABLE" lalu
menugaskan kendaraan yang sama dua kali. `PUT` pada vehicles/drivers MENOLAK
`IN_USE`/`ON_DELIVERY` — status itu bukan milik user untuk diketik.

**Ketersediaan ditegakkan saat dispatch, bukan saat create.** Surat jalan boleh
dijadwalkan lebih dulu selagi kendaraannya masih mengantar yang lain; yang
ditolak saat create hanyalah kendaraan `MAINTENANCE` dan pengemudi `INACTIVE`.

## Integrasi ke ecommerce-service

Arah panggilan SENGAJA `fleet → ecommerce`, bukan sebaliknya: kalau
`ecommerce-service` yang memanggil `fleet-service` saat order SHIPPED, jalur
kritis checkout ikut gagal setiap kali fleet-service mati. Di arah ini
`ecommerce-service` tidak tahu sama sekali soal `fleet-service`, persis seperti
`warehouse-service` tidak tahu soal `ecommerce-service`.

| Titik | Panggilan | Efek |
|---|---|---|
| `POST /delivery-orders` dengan `ecommerce_order_id` | `GET /orders/{id}` | Validasi order ada, milik company yang sama, dan berstatus `SHIPPED`; nomor order + nama/alamat penerima di-SNAPSHOT ke surat jalan. Input manual menang atas snapshot (pengiriman bisa dialihkan ke alamat lain). |
| `POST /delivery-orders/{id}/deliver` | `POST /orders/{id}/deliver` | Order e-commerce ikut maju ke `DELIVERED`. |

Panggilan lintas service dilakukan SEBELUM commit perubahan lokal (pola
identik `shipOrder` di ecommerce-service): kalau `ecommerce-service` menolak,
surat jalan tetap `DISPATCHED` dan kendaraan tetap `IN_USE` — tidak ada state
setengah jadi yang perlu dibereskan manual.

Surat jalan TANPA `ecommerce_order_id` tetap sah (pengiriman internal antar
gudang, kurir dokumen) dan tidak menyentuh `ecommerce-service` sama sekali.

`cancel` SENGAJA tidak mengubah order e-commerce: membatalkan surat jalan
berarti pengirimannya dijadwalkan ulang, bukan bahwa order-nya batal.

## Belum ada / batasan yang disengaja

- **Fact table dw-service + chart BI sudah ada** sejak 2026-08-16
  (`fact_fleet_delivery_orders` dengan grain satu baris per surat jalan,
  endpoint `GET /analytics/fleet-delivery-monthly-summary` di dw-service, dan
  chart "Pengiriman per Bulan" di BI Dashboards, termasuk rata-rata lama
  pengiriman berangkat-sampai-tiba). Yang belum: Materialized View, dan
  rekap per kendaraan/pengemudi (endpoint yang ada meringkas per bulan).
- **Tidak ada rute/optimasi multi-drop** — satu surat jalan = satu tujuan.
  Multi-drop butuh tabel baris tujuan tersendiri dan urutan pemberhentian,
  di luar lingkup modul inti ini.
- **Tidak ada pelacakan posisi real-time** — `iot-service` sudah punya pipeline
  MQTT yang cocok untuk itu kalau nanti dibutuhkan, tapi keduanya sengaja tidak
  disambungkan di fase ini.
- **Kapasitas kendaraan tidak divalidasi terhadap isi order** — `capacity_kg`
  murni informasi; tidak ada pengecekan berat muatan karena `order_items`
  tidak menyimpan berat produk.
- Nomor surat jalan digenerate via `COUNT(*)+1` per company per periode
  (`nextSequence`, disalin dari ecommerce-service/sales-service) — cukup untuk
  dev/demo, bukan production-grade di bawah concurrency tinggi (batasan sama
  yang sudah didokumentasikan di `finance-service/README.md`).
