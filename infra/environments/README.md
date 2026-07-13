# Environment configs (staging / production)

Setiap service Go di platform ini sudah membaca seluruh konfigurasinya lewat
environment variable (pola `getEnv(key, fallback)` di tiap
`internal/config/config.go`) — jadi tidak ada kode yang perlu diubah untuk
pindah environment, cukup env var yang di-override. `localhost`/`change-me`
yang jadi default cuma dipakai kalau env var-nya tidak diset, dan itu memang
sengaja dibuat hanya cocok untuk dev lokal di satu mesin (lihat
`infra/docker-compose.yml`, yang secara struktural adalah alat dev lokal —
`host.docker.internal`, container DNS antar service dalam satu network,
semuanya asumsi satu host — bukan representasi topologi staging/prod
sungguhan).

## Isi folder ini

- `staging.env.example`, `production.env.example` — daftar LENGKAP env var
  yang dibutuhkan seluruh 14 service + frontend, dikelompokkan per service,
  dengan placeholder `REPLACE_ME_...` di tempat yang wajib diisi manual
  (hostname database/Kafka/Redis/ClickHouse yang sesungguhnya, domain
  service-to-service, dan terutama **secret**). Ini template untuk diisi
  nanti kalau infrastruktur staging/prod sungguhan (server, managed Postgres,
  Kafka cluster, dst) sudah ada — **belum ada infrastruktur seperti itu untuk
  proyek ini saat ini**, jadi file ini murni referensi/kerangka, bukan
  konfigurasi yang langsung bisa dipakai.

## Kenapa bukan cuma `docker-compose.yml` dengan `--env-file`

`docker-compose.yml` di folder ini didesain untuk menjalankan SEMUA service
di SATU mesin (dev lokal) — nama container dipakai sebagai hostname
antar-service, dan Postgres dijangkau lewat `host.docker.internal` karena dia
native di host yang sama. Di staging/prod sungguhan, service-service ini
kemungkinan besar akan:
- Berjalan sebagai container/pod terpisah di host/cluster berbeda (lihat
  `infra/kubernetes/` — manifest-nya masih placeholder, menyusul).
- Terhubung ke Postgres/Kafka/Redis/ClickHouse yang benar-benar remote
  (managed service atau cluster sendiri), bukan `host.docker.internal`.
- Saling memanggil lewat DNS internal cluster atau ingress URL sungguhan,
  bukan nama container docker-compose.

Jadi env var untuk staging/prod TIDAK dimuat lewat `docker-compose.yml` yang
sama seperti dev lokal — file `.env.example` di folder ini adalah kerangka
yang nanti dipakai lewat mekanisme deployment sungguhan (K8s Secret/ConfigMap,
systemd `EnvironmentFile=`, `docker run --env-file`, dst — tergantung pilihan
platform deployment yang belum diputuskan).

## Keamanan

- **`JWT_SECRET`** (dipakai `auth-service` untuk menandatangani token, dan
  `api-gateway` untuk memverifikasinya — HARUS sama persis di keduanya) kini
  punya guard di kode: kalau `APP_ENV` bukan `development` DAN `JWT_SECRET`
  masih default `change-me`, service menolak start (`log.Fatalf`) alih-alih
  diam-diam berjalan dengan token yang bisa dipalsukan siapa pun yang baca
  source code ini. Lihat `internal/config/config.go` di `auth-service` dan
  `api-gateway`.
- Set `APP_ENV=staging` atau `APP_ENV=production` (bebas string apa pun
  selain `development`) supaya guard ini aktif. Default `APP_ENV` kalau tidak
  diset adalah `development` (guard tidak aktif, supaya `go run`/Docker lokal
  tetap bisa jalan tanpa perlu set `JWT_SECRET` setiap saat).
- `DATABASE_URL` di template staging/prod pakai `sslmode=require` (bukan
  `disable` seperti default dev) — Postgres managed pada umumnya
  mewajibkan TLS.
- Jangan commit file `.env` hasil isian sungguhan (bukan `.example`) ke git —
  `.gitignore` root sudah mengecualikan pola `*.env` selain `*.env.example`
  (lihat root `.gitignore`, tambahkan kalau belum ada).
