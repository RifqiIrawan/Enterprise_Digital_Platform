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

Setup sekali jalan di Postgres native:
```sql
CREATE ROLE platform LOGIN PASSWORD 'platform' SUPERUSER;
CREATE DATABASE auth_service;
CREATE DATABASE rbac_service;
CREATE DATABASE company_service;
CREATE DATABASE audit_service;
```
Lalu jalankan file di `backend/services/<service>/migrations/*.sql` secara berurutan
per database (lihat urutan angka di nama filenya).

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
| Kafka | 9092 | Event streaming (KRaft mode, tanpa Zookeeper) |
| Kafka UI | 8090 | Dashboard untuk inspeksi topic/consumer |

Matikan semua: `./scripts/dev-down.ps1`

## Struktur
```
infra/
├── docker-compose.yml
├── kafka/topics.md         # konvensi penamaan topic
├── kubernetes/             # placeholder manifest (fase deployment K8s)
│   ├── base/
│   └── overlays/{dev,staging,prod}/
└── scripts/                 # dev-up.ps1, dev-down.ps1
```

## Status
Fase 1 — docker-compose untuk local dev. Manifest Kubernetes menyusul sesuai `14_Kubernetes_Deployment.md`.
