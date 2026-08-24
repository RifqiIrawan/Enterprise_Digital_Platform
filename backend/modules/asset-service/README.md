# Asset Service

Modul aset: pendataan & maintenance (Fase 2), penyusutan & kalibrasi (Fase 5).
Role terkait: `Asset`. Event dipublikasikan ke Kafka dengan prefix `asset.*`.

Aset di sini adalah barang milik perusahaan (mesin, kendaraan, peralatan), bukan
barang dagangan: tidak ada product master dan tidak ada mutasi stok.
`warehouse_id` hanya menandai lokasi fisik dan tidak memicu apa pun di
warehouse-service.

## Penyusutan

Mengikuti pola payroll di hr-service: satu **run per periode (bulan)** yang
dihitung dulu (DRAFT), lalu diposting ke buku besar lewat finance-service.

```
Garis lurus      : (harga perolehan - nilai residu) / umur manfaat (bulan)
Saldo menurun    : nilai buku x (2 / umur manfaat)
```

- **Tidak ada proporsi hari.** Aset yang mulai disusutkan tanggal berapa pun
  dalam sebuah bulan dikenai satu bulan penuh.
- **Bulan terakhir dipotong** supaya nilai buku berhenti tepat di nilai residu.
- Aset ikut dihitung kalau `useful_life_months` terisi, `depreciation_start_date`
  sudah lewat atau sama dengan akhir periode, statusnya bukan `DISPOSED`, dan
  nilai bukunya masih di atas nilai residu.
- `accumulated_depreciation` di tabel `assets` **hanya** naik saat run diposting;
  `book_value` tidak disimpan, selalu diturunkan dari keduanya.

Urutan periode dijaga dua penolakan: run baru ditolak selama masih ada run
DRAFT (perhitungan berikutnya akan memakai akumulasi yang belum termasuk run
itu), dan ditolak untuk periode yang sama atau lebih lama dari run POSTED
terakhir (akumulasi adalah angka berjalan). Run DRAFT boleh dihapus dan
dihitung ulang; run POSTED tidak.

Postingnya satu jurnal per periode — debit Beban Penyusutan, kredit Akumulasi
Penyusutan, bertanggal akhir periode, `reference_type = ASSET_DEPRECIATION`.
Rinciannya per aset tetap tersimpan di `depreciation_entries`. finance-service
dipanggil dulu; akumulasi aset baru naik setelah jurnalnya berhasil, sehingga
kegagalan finance-service meninggalkan run tetap DRAFT dan bisa diulang.

## Kalibrasi

Kewajiban berulang, bukan kejadian sekali:

- Hasil **PASS/ADJUSTED** dengan `interval_months` terisi langsung menjadwalkan
  kalibrasi berikutnya pada `performed_date + interval`.
- Hasil **FAIL** menarik status aset ke `MAINTENANCE` dan **tidak** menjadwalkan
  ulang: alat yang gagal kalibrasi butuh perbaikan dulu, dan tanggal berikutnya
  tergantung kapan itu selesai.
- "Terlambat" dihitung saat ditampilkan (`scheduled_date` lewat tapi masih
  `SCHEDULED`), bukan status tersendiri yang butuh job terjadwal.

## Endpoint

| Method | Path | Catatan |
|---|---|---|
| GET/POST | `/assets`, `/assets/{id}` (PUT) | field penyusutan opsional di PUT: yang tidak dikirim tidak diubah, `useful_life_months: 0` mengosongkannya |
| GET/POST | `/maintenance-schedules` | + `/{id}/complete`, `/{id}/cancel` |
| GET/POST | `/depreciation-runs`, `/depreciation-runs/{id}` | perhitungan per periode (DRAFT) |
| POST | `/depreciation-runs/{id}/post` | body: `expense_account_id`, `accumulated_depreciation_account_id` |
| DELETE | `/depreciation-runs/{id}` | hanya DRAFT |
| GET/POST | `/calibrations` | filter: `asset_id`, `status`, `due_before` |
| POST | `/calibrations/{id}/complete`, `/calibrations/{id}/cancel` | |

## Konfigurasi

Selain `PORT`, `DATABASE_URL`, `KAFKA_BROKERS`, dan `OTLP_ENDPOINT`, service ini
butuh `FINANCE_SERVICE_URL` (default `http://localhost:8085`) untuk memposting
jurnal penyusutan — panggilan langsung service-to-service, tidak lewat gateway.

## Tes

```
go test ./...
```

Butuh Postgres lokal (`postgres://platform:platform@localhost:5432`); tanpa itu
seluruh test di-skip, bukan gagal. Database `asset_service_test` dibuat dan
dimigrasikan otomatis. finance-service di-stub lewat httptest.
