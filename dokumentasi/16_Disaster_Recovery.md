# 16 — Disaster Recovery
## Enterprise Digital Platform (EDP)

---

## Overview

Strategi DR EDP untuk lingkungan development saat ini adalah **sederhana dan pragmatis** — fokus pada recovery cepat dari kegagalan umum (service crash, DB corruption, infra down), bukan enterprise-grade DR dengan RTO/RPO formal.

---

## Komponen dan Ketahanannya

### PostgreSQL (native Windows service)

**Persistence**: Data tersimpan di disk lokal (`C:\Program Files\PostgreSQL\18\data\`).  
**Recovery dari crash**: Service restart otomatis (Windows service manager).  
**Recovery dari corruption**: Restore dari backup terakhir.

**Backup manual**:
```bash
# Backup semua database EDP
for db in auth_service company_service rbac_service audit_service finance_service \
          hr_service sales_service purchasing_service warehouse_service \
          production_service qc_service asset_service iot_service; do
  pg_dump -U platform $db > backup_${db}_$(date +%Y%m%d).sql
done

# Restore
psql -U platform $db < backup_${db}_YYYYMMDD.sql
```

Tidak ada WAL-G, tidak ada point-in-time recovery, tidak ada automated backup schedule untuk dev environment.

### Kafka (Docker container)

**Persistence**: Volume `kafka_data` di Docker.  
**Recovery**: `docker compose up -d kafka` — data tetap ada kalau volume tidak dihapus.  
**Event loss**: Kalau volume dihapus, semua event hilang — tidak ada replikasi, tidak ada backup.

**Catatan**: Semua event yang masuk ke audit-service sudah disimpan ke Postgres (`audit_logs`). ClickHouse (via dw-service) juga menyimpan data yang sudah sync. Kehilangan Kafka berarti event yang **belum dikonsumsi** hilang, bukan semua data.

### ClickHouse (Docker container)

**Persistence**: Volume `clickhouse_data` di Docker.  
**Recovery**: `docker compose up -d clickhouse` — data tetap.  
**Backup**:
```bash
# Export fact table ke file
clickhouse-client --query "SELECT * FROM dw.fact_finance_journal_lines FORMAT JSONEachRow" > backup_finance_$(date +%Y%m%d).jsonl

# Re-sync dari Postgres kalau data hilang
curl -X POST http://localhost:8095/sync
```

Re-sync `POST /sync` akan mengisi ulang ClickHouse dari Postgres karena watermark direset (atau bisa diset manual di `etl_sync_state`).

### MinIO (Docker container)

**Persistence**: Volume `minio_data` di Docker.  
**Recovery**: `docker compose up -d minio` — data tetap.  
**Catatan**: MinIO lake adalah bronze layer best-effort — kehilangannya tidak mempengaruhi sistem utama (Postgres + ClickHouse).

### Redis (Docker container)

**Persistence**: Tidak ada (in-memory only di config default).  
**Recovery**: `docker compose up -d redis` — data hilang, tapi hanya session/cache yang affected. User perlu login ulang.

---

## Recovery Scenarios

### 1. Service crash (Go binary)

```bash
# Restart service
cd backend/modules/finance-service
go run ./cmd/server

# Atau via docker-compose
docker compose restart finance-service
```

Migrasi jalan otomatis saat restart — aman.

### 2. Infra Docker down

```bash
cd infra
docker compose up -d
```

Cek port yang mungkin bentrok sebelum up (lihat Known Issues di NEXT_SESSION.md).

### 3. Database loss (rare)

Jika database hilang karena corruption atau accidental drop:
```bash
# Buat ulang database
psql -U postgres -c "CREATE DATABASE finance_service OWNER platform;"

# Jalankan service — migrasi otomatis akan buat semua tabel
cd backend/modules/finance-service
go run ./cmd/server
```

Data hilang, harus diisi ulang dari backup atau manual entry.

### 4. ClickHouse data loss

```bash
# Restart ClickHouse
docker compose up -d clickhouse

# Re-sync semua data dari Postgres
curl -X POST http://localhost:8095/sync
```

`POST /sync` mengambil ulang semua data dari Postgres (bukan hanya incremental) kalau watermark di `etl_sync_state` di-reset atau tidak ada.

---

## Checklist Sebelum Sesi Development

```bash
# 1. Pastikan Postgres Windows service jalan
Get-Service postgresql-x64-18  # PowerShell

# 2. Jalankan infra Docker
cd infra && docker compose up -d

# 3. Cek tidak ada port konflik dengan project lain
curl http://localhost:8082/health  # cek field "service" di response
curl http://localhost:8085/health  # field "service" harus "finance-service"

# 4. Jalankan semua service Go (di terminal terpisah atau via process manager)

# 5. Health check semua service
for port in 8079 8081 8082 8083 8084 8085 8086 8087 8088 8089 8090 8091 8092 8093 8094 8095; do
  echo -n "$port: "; curl -s http://localhost:$port/health | python -m json.tool 2>/dev/null || echo "DOWN"
done
```
