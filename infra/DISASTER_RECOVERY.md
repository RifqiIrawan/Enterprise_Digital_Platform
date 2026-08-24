# Disaster Recovery (Fase 12)

Dokumen ini menjawab satu pertanyaan: **kalau datanya hilang, bagaimana
mendapatkannya kembali, dan seberapa banyak yang tetap hilang?**

## Apa yang perlu di-backup, dan apa yang tidak

| Penyimpanan | Di-backup? | Alasan |
|---|---|---|
| **Postgres** (18 database `*_service`) | **Ya** — `infra/scripts/backup-postgres.ps1` | Satu-satunya sumber kebenaran di platform ini. Semua yang lain diturunkan darinya. |
| ClickHouse (`fact_*`, 16 fact table) | Tidak | **Data turunan.** dw-service membangunnya dari Postgres lewat ETL; yang perlu diselamatkan adalah sumbernya, bukan hasilnya. Lihat "Membangun ulang ClickHouse" di bawah. |
| MinIO (data lake bronze) | Tidak | Sama: dual-write dari ETL yang sama, isinya salinan mentah dari Postgres. |
| Kafka | Tidak | Alat angkut, bukan penyimpan. Event yang hilang bukan data yang hilang — audit trail-nya sudah mendarat di `audit_service` (Postgres). |
| Redis | Tidak | Cache & session. Hilang = orang login ulang. |
| Prometheus / Loki / Jaeger | Tidak | Data operasional dev-only dengan retensi pendek; kalau nanti dipakai sungguhan, retensinya diputuskan tersendiri. |

Kesimpulan yang penting: **cakupan backup platform ini = 18 database Postgres.**
Itu bukan penyederhanaan yang malas, melainkan konsekuensi dari arsitekturnya —
dan alasan kenapa satu skrip sudah cukup.

## Backup

```powershell
$env:PGPASSWORD = "platform"
./infra/scripts/backup-postgres.ps1                 # ke <repo>/backups/<timestamp>/
./infra/scripts/backup-postgres.ps1 -OutputRoot D:\edp-backups -KeepDays 30
```

- Satu berkas `.dump` per database (format custom `-Fc`), plus `manifest.json`
  berisi ukuran, durasi, dan **sha256** tiap berkas.
- Database `*_test` dilewati: isinya dibuat ulang tiap kali `go test` jalan.
- Daftar database ditemukan dari server, bukan ditulis tetap di skrip — modul
  baru otomatis ikut ter-backup.
- **Gagal satu, gagal semua** (exit 1). Backup yang melaporkan sukses padahal
  sebagian gagal lebih berbahaya daripada tidak ada backup: yang pertama
  membuat orang berhenti khawatir.
- `backups/` ada di `.gitignore`. Dump berisi data sungguhan.

## Pemulihan

```powershell
$env:PGPASSWORD = "platform"

# Latihan (aman): pulihkan ke nama lain, lalu bandingkan isinya.
./infra/scripts/restore-postgres.ps1 `
    -DumpFile .\backups\2026-08-25_053634\rbac_service.dump `
    -TargetDatabase rbac_service_dr_drill

# Sungguhan: menimpa database yang sedang ada (butuh -Force).
./infra/scripts/restore-postgres.ps1 `
    -DumpFile .\backups\2026-08-25_053634\finance_service.dump `
    -TargetDatabase finance_service -Force
```

Penjaga yang ada di skrip pemulihan:

1. **sha256 dicocokkan dengan manifest** sebelum memulihkan — memulihkan dari
   berkas yang rusak separuh menghasilkan database yang *terlihat* pulih.
2. **Database yang sudah ada tidak ditimpa tanpa `-Force`.**
3. Satu database per perintah. Tidak ada mode "pulihkan semuanya": perintah
   yang bisa menimpa 18 database sekaligus cepat atau lambat akan dijalankan
   di database yang salah.
4. Setelah selesai, jumlah tabel dilaporkan dari database hasil pemulihan —
   bukan sekadar exit code `pg_restore`, yang juga bernilai 1 untuk peringatan
   tidak fatal.

## Latihan pemulihan yang sudah dilakukan

**Backup yang tidak pernah dipulihkan belum terbukti bisa dipulihkan.** Karena
itu jalurnya dicoba sungguhan, bukan cuma ditulis:

| Langkah | Hasil (2026-08-25) |
|---|---|
| Backup seluruh database | 18/18 berhasil, total 0,3 MB |
| Verifikasi sha256 dari manifest | cocok |
| Pulihkan `rbac_service` → `rbac_service_dr_drill` | 7 tabel, 1,8 detik |
| Bandingkan isi sumber vs hasil | `modules=17 menus=59 roles=21 perms=290 migrations=18` — **identik** |
| Bersihkan database latihan | dihapus |

Ulangi latihan ini setiap kali skema berubah besar, dan setelah setiap
perubahan pada skrip backup/restore.

## Skenario

### 1. Satu service datanya rusak/terhapus

Pulihkan database itu saja dengan `-Force`. Service lain tidak perlu disentuh —
itulah gunanya dump per database.

### 2. Seluruh Postgres hilang

Pasang Postgres, buat user `platform`, lalu jalankan `restore-postgres.ps1`
untuk tiap berkas `.dump` di folder backup terakhir. Service akan menjalankan
migrasinya sendiri saat start (`store.Migrate`), jadi urutan start tidak
kritis.

**RPO** = jarak antar-backup (sekarang: manual, jadi sebesar jarak terakhir
kali dijalankan). **RTO** ≈ menit untuk data sebesar dev ini; untuk data
produksi, ukur ulang dengan latihan pemulihan di atas.

### 3. ClickHouse hilang — membangun ulang, bukan memulihkan

ClickHouse tidak di-backup. Bangun ulang dari Postgres:

1. Jalankan dw-service; `EnsureSchema` membuat kembali seluruh tabel & MV.
2. Kosongkan watermark ETL (tabel watermark di ClickHouse) supaya `POST
   /api/dw/sync` menarik dari awal, bukan hanya data baru — ETL-nya
   incremental lewat watermark (`GetWatermark`/`SetWatermark` di
   `internal/etl/*.go`), jadi tanpa dikosongkan dia akan mengira sudah
   sinkron.
3. `POST /api/dw/sync`, lalu bandingkan cacah baris fact table dengan
   sumbernya.

Konsekuensinya: **data yang sudah dihapus dari Postgres tidak akan kembali di
ClickHouse.** Untuk platform ini itu perilaku yang diinginkan (gudang data
mengikuti sumber), tapi harus disadari sebelum mengandalkan ClickHouse sebagai
arsip jangka panjang — dia bukan arsip.

### 4. MinIO (data lake bronze) hilang

Sama seperti ClickHouse: dual-write dari ETL yang sama. Kosongkan watermark,
sinkron ulang.

## Yang belum ada (jujur disebut)

- **Penyimpanan offsite.** Backup sekarang mendarat di disk yang sama dengan
  databasenya. Satu disk mati = keduanya hilang. Ini kelemahan terbesar yang
  tersisa; perbaikannya (salin ke MinIO/S3 atau disk lain) belum dikerjakan.
- **Enkripsi.** Dump disimpan apa adanya. Berisi data pegawai & keuangan.
- **Penjadwalan.** Belum ada Task Scheduler/cron; skripnya masih dijalankan
  manual, jadi RPO-nya belum punya angka pasti.
- **Uji pemulihan otomatis.** Latihan di atas dikerjakan manual; idealnya
  dijalankan berkala oleh mesin yang juga memeriksa hasil bandingannya.
- **Backup ClickHouse/MinIO tetap tidak direncanakan** — pembangunan ulangnya
  sudah cukup selama Postgres selamat, dan itu keputusan sadar, bukan
  kelalaian.
