# MinIO / S3 untuk data lake dw-service

Bucket `dw-lake` berisi tiga lapis: Bronze (`<fact>/YYYY/MM/DD/*.jsonl`),
`silver/` dan `gold/`. Lihat `dokumentasi/06_Data_Lake_Architecture.md`.

## Akun least-privilege

`dw-service-policy.json` adalah policy minimum untuk akun dw-service di
production (jangan pakai root). Diverifikasi terhadap MinIO sungguhan: seluruh
suite `internal/datalake` dan `internal/httpapi` lolos dengan akun ini, dan akun
yang sama ditolak (`Access Denied`) di bucket lain.

```sh
mc admin policy create prod dw-service dw-service-policy.json
mc admin user add prod dw-service <SECRET>
mc admin policy attach prod dw-service --user dw-service
```

Bucket harus sudah dibuat oleh admin (akun ini sengaja tidak boleh
`s3:CreateBucket`; `Connect` hanya membuat bucket kalau belum ada, jadi di
production cukup pastikan bucket-nya ada). Isi `MINIO_ACCESS_KEY` /
`MINIO_SECRET_KEY` di secret store, bukan di repo; `MINIO_USE_SSL=true`.

## Retention: sengaja BELUM ada expiry Bronze

Jangan pasang lifecycle expiry di Bronze. Build **penuh** Silver (tiap 24 jam,
saat recovery, atau `?full=true`) membaca seluruh Bronze. Baris yang hanya
hidup di objek Bronze yang sudah kedaluwarsa hilang dari Silver, dan
`GET /api/dw/lake/reconcile` melaporkan `MISSING_FROM_LAKE`. Retention Bronze
baru aman setelah ada pemadatan Bronze (lihat "Perbaikan yang bisa dilakukan"
di dokumen arsitektur lake).

## Upload multipart yang yatim

Merge Silver memakai multipart upload streaming. Build yang mati di tengah
jalan meninggalkan bagian upload. Aturan lifecycle `AbortIncompleteMultipartUpload`
**ditolak MinIO** (dicoba di `RELEASE.2025-09-07`: "XML ... did not validate");
MinIO membersihkannya sendiri lewat `api stale_uploads_expiry` (default 24 jam).
Di AWS S3 sungguhan aturan itu dibutuhkan, tetapi stack ini memakai MinIO.
