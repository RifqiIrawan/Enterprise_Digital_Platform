# Infra

Infrastruktur lokal & deployment untuk Enterprise Digital Platform.

## PostgreSQL — jalan native, bukan Docker

PostgreSQL sengaja **tidak** ada di `docker-compose.yml`. Di lingkungan dev ini ia
jalan sebagai Windows service native (`postgresql-x64-18`, port 5432) — lebih
stabil daripada lewat Docker Desktop yang di mesin ini pernah macet
(engine pipe error 500) saat memori menipis akibat banyak proyek Docker lain
berjalan bersamaan. Kalau mesin lain tidak punya kendala serupa, silakan
jalankan Postgres lewat Docker seperti biasa — cukup tambahkan kembali service
`postgres` di `docker-compose.yml` mengikuti pola service lain di file ini.

Setup sekali jalan di Postgres native — **seluruh 13 database** (4 dari Fase 1
+ 9 dari Fase 2; `ai-bi-service` tidak punya database sendiri, murni agregasi
HTTP) wajib sudah ada sebelum service manapun (native atau container) bisa
start, karena migrasi jalan otomatis saat startup tapi tidak bisa membuat
database-nya sendiri:
```sql
CREATE ROLE platform LOGIN PASSWORD 'platform' SUPERUSER;
CREATE DATABASE auth_service;
CREATE DATABASE rbac_service;
CREATE DATABASE company_service;
CREATE DATABASE audit_service;
CREATE DATABASE finance_service;
CREATE DATABASE hr_service;
CREATE DATABASE sales_service;
CREATE DATABASE purchasing_service;
CREATE DATABASE warehouse_service;
CREATE DATABASE production_service;
CREATE DATABASE qc_service;
CREATE DATABASE asset_service;
```
Migrasi tiap service (embed FS + tabel `schema_migrations`, aman dijalankan
berkali-kali) jalan otomatis begitu service-nya start — tidak perlu
menjalankan file `migrations/*.sql` secara manual.

## Local development (ClickHouse, Redis, MinIO, Kafka)
```
cp .env.example .env
./scripts/dev-up.ps1      # atau: docker compose up -d
```

Service yang naik:
| Service | Host Port | Keterangan |
|---|---|---|
| PostgreSQL | 5432 | **Native**, bukan Docker — lihat di atas. Satu database per service (`auth_service`, `company_service`, `rbac_service`, `audit_service`, dst) |
| ClickHouse | 8123 (HTTP), 9101 (native) | OLAP / data warehouse, lihat `05_Data_Warehouse_Architecture.md` |
| Redis | 6379 | Cache, session, rate limiting |
| MinIO | 9002 (API), 9003 (Console) | Object storage (S3-compatible) |
| Kafka | 9092 (host/native clients), internal listener `kafka:29092` (container clients, lihat di bawah) | Event streaming (KRaft mode, tanpa Zookeeper) |
| Kafka UI | 8099 | Dashboard untuk inspeksi topic/consumer (host 8090 dipakai production-service) |

Matikan semua: `./scripts/dev-down.ps1`

## Full app stack di Docker (opsional, di atas infra)

`docker-compose.yml` juga bisa menjalankan seluruh 14 service Go + frontend
sebagai container, bukan cuma infra di atas — berguna untuk mendekati
deployment production tanpa perlu 15 terminal `go run`/`npm run dev` manual.
Prasyarat sama seperti local dev: Postgres native harus sudah jalan dengan
ke-13 database di atas sudah dibuat.

```
cd infra
docker compose up -d --build
```

Karena Postgres tetap native (bukan container), semua service app
terhubung ke sana lewat hostname khusus Docker Desktop
`host.docker.internal` (bukan `localhost`, yang di dalam container merujuk
ke container itu sendiri) — sudah di-wire di `docker-compose.yml` lewat
env var `DATABASE_URL` + `extra_hosts`. Kafka juga punya listener kedua
(`kafka:29092`, internal-only) khusus untuk container, terpisah dari
listener `localhost:9092` yang dipakai proses native — tanpa ini, metadata
response Kafka akan mengarahkan container client untuk reconnect ke
"localhost" versi dirinya sendiri dan produce/consume diam-diam gagal.

Port host yang dipetakan sama persis dengan menjalankan tiap service native
(lihat tabel port di `NEXT_SESSION.md` bagian "Cara Menjalankan") — jadi
frontend tetap diakses di `http://localhost:3000` dan API gateway di
`http://localhost:8079`, baik service-nya jalan native maupun sebagai
container.

Rebuild image setelah ganti kode: `docker compose build <nama-service>` lalu
`docker compose up -d <nama-service>`. Mematikan seluruhnya (app + infra):
`docker compose down`.

## Struktur
```
infra/
├── docker-compose.yml       # infra (Kafka/Redis/MinIO/ClickHouse) + seluruh app stack (opsional)
├── kafka/topics.md          # konvensi penamaan topic
├── kubernetes/               # placeholder manifest (fase deployment K8s)
│   ├── base/
│   └── overlays/{dev,staging,prod}/
└── scripts/                 # dev-up.ps1, dev-down.ps1

# Dockerfile tiap service ada di direktorinya sendiri, bukan di sini:
backend/services/<service>/deployments/Dockerfile
backend/modules/<module>/deployments/Dockerfile
frontend/web/Dockerfile
```

## Status
Fase 1 — docker-compose untuk local dev infra SELESAI. Dockerfile + docker-compose
untuk full app stack (14 service Go + frontend) SELESAI. Manifest Kubernetes
masih placeholder, menyusul sesuai `14_Kubernetes_Deployment.md`. CI/CD dan
environment config staging/prod (di luar `docker-compose.yml` lokal ini) juga
belum dikerjakan.
