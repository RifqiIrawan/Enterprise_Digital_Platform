# 20 — Implementation Guide
## Enterprise Digital Platform (EDP)

---

## Prerequisites

| Tool | Versi | Catatan |
|------|-------|---------|
| Go | 1.25.0+ | Install dari golang.org |
| Node.js | 20+ | Untuk frontend |
| PostgreSQL | 18 | Windows service atau container |
| Docker Desktop | Latest | Untuk Kafka, Redis, ClickHouse, MinIO, Mosquitto |
| Git | Any | |

---

## Setup Pertama Kali

### 1. Clone Repository

```bash
git clone https://github.com/RifqiIrawan/Enterprise_Digital_Platform.git
cd Enterprise_Digital_Platform
```

### 2. Setup PostgreSQL

Pastikan PostgreSQL 18 jalan (Windows service `postgresql-x64-18`) dengan role `platform`:

```sql
-- Di psql sebagai postgres user
CREATE ROLE platform WITH LOGIN PASSWORD 'platform';
ALTER ROLE platform CREATEDB;

-- Buat 13 database untuk semua service
CREATE DATABASE auth_service OWNER platform;
CREATE DATABASE company_service OWNER platform;
CREATE DATABASE rbac_service OWNER platform;
CREATE DATABASE audit_service OWNER platform;
CREATE DATABASE finance_service OWNER platform;
CREATE DATABASE hr_service OWNER platform;
CREATE DATABASE sales_service OWNER platform;
CREATE DATABASE purchasing_service OWNER platform;
CREATE DATABASE warehouse_service OWNER platform;
CREATE DATABASE production_service OWNER platform;
CREATE DATABASE qc_service OWNER platform;
CREATE DATABASE asset_service OWNER platform;
CREATE DATABASE iot_service OWNER platform;
```

### 3. Jalankan Infra Docker

```bash
cd infra
docker compose up -d
```

Services yang naik: Kafka (port 9092), Kafka UI (8099), Redis (6379), ClickHouse (9101 native, 8123 HTTP), MinIO (9004 API, 9001 Console), Mosquitto (1883).

**Jika image Bitnami gagal pull**: pastikan pakai `bitnamilegacy/kafka:3.7.1` di `docker-compose.yml` (bukan `bitnami/kafka` yang sudah berbayar).

### 4. Jalankan Backend Services

Buka 16 terminal (atau pakai process manager). Urutan yang disarankan:

```bash
# Terminal 1: api-gateway
cd backend/services/api-gateway && go run ./cmd/server

# Terminal 2-5: Platform services
cd backend/services/auth-service && go run ./cmd/server
cd backend/services/company-service && go run ./cmd/server
cd backend/services/rbac-service && go run ./cmd/server
cd backend/services/audit-service && go run ./cmd/server

# Terminal 6-16: Business modules
cd backend/modules/finance-service && go run ./cmd/server
cd backend/modules/hr-service && go run ./cmd/server
cd backend/modules/sales-service && go run ./cmd/server
cd backend/modules/purchasing-service && go run ./cmd/server
cd backend/modules/warehouse-service && go run ./cmd/server
cd backend/modules/production-service && go run ./cmd/server
cd backend/modules/qc-service && go run ./cmd/server
cd backend/modules/asset-service && go run ./cmd/server
cd backend/modules/ai-bi-service && go run ./cmd/server
cd backend/modules/iot-service && go run ./cmd/server
cd backend/modules/dw-service && go run ./cmd/server
```

Migrasi database berjalan **otomatis** saat startup pertama.

### 5. Jalankan Frontend

```bash
cd frontend/web
npm install
npm run dev
```

Frontend tersedia di `http://localhost:3000`.

### 6. Verifikasi

```bash
# Cek semua service hidup
for port in 8079 8081 8082 8083 8084 8085 8086 8087 8088 8089 8090 8091 8092 8093 8094 8095; do
  echo -n "Port $port: "
  curl -s http://localhost:$port/health | python -c "import sys,json; d=json.load(sys.stdin); print(d['service'])" 2>/dev/null || echo "DOWN"
done
```

### 7. Login

Buka `http://localhost:3000` dan login dengan:
- **Email**: `admin@edp.local`
- **Password**: `Admin@12345`

---

## Cara Menambah Modul Bisnis Baru

### Backend

1. **Buat struktur folder** di `backend/modules/{nama}-service/`:
   ```
   cmd/server/main.go
   internal/config/config.go
   internal/model/{domain}.go
   internal/store/store.go
   internal/eventbus/eventbus.go
   internal/httpapi/handler.go
   internal/httpapi/{domain}.go
   migrations/embed.go
   migrations/001_init.sql
   go.mod
   ```
   Salin dari `finance-service` sebagai template.

2. **Daftarkan di** `backend/go.work`:
   ```
   use (
       ...
       ./modules/{nama}-service
   )
   ```

3. **Tambahkan route di** `backend/services/api-gateway/internal/gateway/gateway.go`:
   ```go
   {NamaServiceURL: getEnv("{NAMA}_SERVICE_URL", "http://localhost:80XX")}
   // dan tambahkan di slice routes:
   {Prefix: "/api/{nama}/", Target: cfg.NamaServiceURL}
   ```

4. **Buat database**:
   ```sql
   CREATE DATABASE {nama}_service OWNER platform;
   ```

5. **Seed RBAC** di `backend/services/rbac-service/migrations/`:
   ```sql
   -- NNN_seed_{nama}_menus.sql
   INSERT INTO modules (name, display_name, sort_order, is_active)
   VALUES ('{nama}', '{Nama}', 120, true);
   -- dst: roles, menus, role_menu_permissions
   ```

### Frontend

1. **Buat halaman** di `frontend/web/src/pages/{nama}/`.
2. **Tambahkan route** di `App.jsx`.
3. **Sidebar** otomatis muncul dari RBAC (tidak perlu edit sidebar).

### Production Readiness

Setelah modul jalan dan diverifikasi:
1. Tambahkan `deployments/Dockerfile` (salin dari service lain)
2. Tambahkan service di `infra/docker-compose.yml`
3. Tambahkan `{nama}-service.yaml` di `infra/kubernetes/base/` + update `kustomization.yaml`
4. Tambahkan `{nama}-service.env.example` di `infra/environments/staging/` dan `production/`
5. Tambahkan ke matrix di `.github/workflows/backend-ci.yml`

---

## Troubleshooting Umum

### Port konflik
```bash
# Cek siapa yang pakai port tertentu
curl http://localhost:8082/health  # Lihat field "service"
# Kalau bukan "company-service", ada proses lain di port itu
```

### Kafka consumer stuck setelah topic baru
Sudah diperbaiki (commit `c925b0f`) — consumer sekarang recreate Reader on error. Tidak perlu restart manual.

### ClickHouse tidak bisa diakses
```bash
# Image resmi butuh credential eksplisit
docker compose up -d --force-recreate clickhouse
# (perlu --force-recreate kalau container sudah ada sebelum env var ditambahkan)
```

### `go build` gagal di Go workspace
```bash
# Jangan jalankan dari backend/ root
cd backend/modules/finance-service
go build ./...  # ✅ Benar

# Bukan dari sini
cd backend
go build ./...  # ❌ Salah
```

### Docker build gagal — `go.sum not found`
```bash
# Jalankan go mod tidy di dalam folder module individual
cd backend/modules/{nama}-service
go mod tidy
```

---

## Menjalankan Tests

```bash
# Test satu service (butuh Postgres jalan)
cd backend/modules/finance-service
go test ./... -v -count=1

# Test dw-service (butuh Postgres + ClickHouse)
cd backend/modules/dw-service
DW_TEST_CLICKHOUSE_ADDR=localhost:9101 go test ./... -v -count=1

# SKIP otomatis kalau DB tidak tersedia (exit 0, bukan fail)
```

---

## Menjalankan Full Stack via Docker

```bash
cd infra

# Build semua image
docker compose build

# Jalankan 19 container
docker compose up -d

# Verifikasi semua container Up
docker compose ps

# Teardown
docker compose down
```
