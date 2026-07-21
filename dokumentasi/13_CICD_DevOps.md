# 13 — CI/CD & DevOps
## Enterprise Digital Platform (EDP)

---

## Overview

CI/CD EDP menggunakan **GitHub Actions** untuk build, test, dan verifikasi. Tidak ada ArgoCD, tidak ada Helm, tidak ada registry push otomatis — ini adalah CI-only pipeline, CD dilakukan manual.

---

## CI Pipelines

### Backend CI (`.github/workflows/backend-ci.yml`)

**Trigger**: Push/PR ke `master` yang menyentuh `backend/` atau workflow file itu sendiri.

**Services yang dijalankan di CI**:

| Service | Image | Port | Dipakai oleh |
|---------|-------|------|-------------|
| postgres | `postgres:16-alpine` | 5432 | Semua 16 service (integration tests) |
| clickhouse | `clickhouse/clickhouse-server:24.3` | 9101 (native), 8123 (HTTP) | dw-service tests |
| minio | `bitnamilegacy/minio:latest` | 9000, 9001 | dw-service lake tests |
| kafka | `bitnamilegacy/kafka:3.7.1` | 9092 | (opsional, streaming tests SKIP kalau tidak ada) |

> **Catatan**: `bitnami` bukan `minio/minio` — image resmi MinIO butuh argumen CMD eksplisit (`server /data`) yang tidak bisa diset di GitHub Actions `services:` block. Bitnami image default CMD-nya sudah jalan server.

**Matrix strategy**: build/vet/test dijalankan paralel untuk setiap service Go:

```yaml
strategy:
  matrix:
    module:
      - services/api-gateway
      - services/auth-service
      - services/company-service
      - services/rbac-service
      - services/audit-service
      - modules/finance-service
      - modules/hr-service
      - modules/sales-service
      - modules/purchasing-service
      - modules/warehouse-service
      - modules/production-service
      - modules/qc-service
      - modules/asset-service
      - modules/ai-bi-service
      - modules/iot-service
      - modules/dw-service
```

**Setiap matrix leg**:
```yaml
steps:
  - go build ./...     # dari dalam folder module (bukan dari backend/ root)
  - go vet ./...
  - go test ./... -count=1 -timeout=120s
```

> **Penting**: `go build ./...` harus dijalankan dari DALAM folder module, bukan dari `backend/`. Go workspace mode (`go.work`) tidak expand `./...` dari root workspace dengan benar.

**Test behavior**:
- Kalau Postgres tidak reachable → `TestMain` `os.Exit(0)` (SKIP, bukan FAIL)
- Kalau ClickHouse tidak reachable → test ClickHouse SKIP (test Postgres tetap jalan)
- Kalau MinIO tidak reachable → `TestSyncFinance_WritesToDataLake` di-skip via `t.Skip`

### Frontend CI (`.github/workflows/frontend-ci.yml`)

**Trigger**: Push/PR ke `master` yang menyentuh `frontend/web/`.

```yaml
steps:
  - uses: actions/setup-node@v4 (node 20, npm cache)
  - npm ci
  - npm run lint       # ESLint
  - npm run build      # Vite build (verifikasi tidak ada compile error)
```

---

## Docker Build

Setiap service memiliki `deployments/Dockerfile` dengan pola multi-stage:

```dockerfile
# Backend service
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/server /server
EXPOSE 808X
CMD ["/server"]
```

```dockerfile
# Frontend
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
ARG VITE_API_BASE_URL=http://localhost:8079
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf  # SPA fallback: try_files $uri /index.html
EXPOSE 80
```

> **Gotcha Go version**: `go.mod` masing-masing module deklarasi `go 1.25.0`. Dockerfile HARUS pakai `golang:1.25-alpine`, bukan `golang:1.22-alpine` — meskipun `GOTOOLCHAIN=auto` bisa workaround ini, lebih baik eksplisit.

> **Gotcha go.sum**: Setiap module harus punya `go.sum`-nya sendiri (hasil `go mod tidy` di dalam folder module). `go build` via `go.work` bisa jalan tanpa ini, tapi `docker build` (standalone, tanpa workspace) tidak.

---

## Full Stack via Docker Compose

```bash
cd infra
docker compose build   # Build 15 image (14 Go services + 1 frontend)
docker compose up -d   # Jalankan 19 container
```

**Kafka membutuhkan dual listener**:
- `PLAINTEXT://localhost:9092` — untuk proses Go native di host
- `INTERNAL://kafka:29092` — untuk container-to-container

URL lintas-service di docker-compose pakai nama container: `http://finance-service:8085`.

Frontend `VITE_API_BASE_URL` tetap `http://localhost:8079` — karena yang manggil API adalah browser (client-side JS), bukan container.

---

## Deployment ke Kubernetes

Setelah build dan push image ke registry:
```bash
# Dev (Docker Desktop K8s)
kubectl apply -k infra/kubernetes/overlays/dev

# Verifikasi
kubectl get pods -n edp-dev

# Teardown
kubectl delete -k infra/kubernetes/overlays/dev
```

Detail K8s manifest di `14_Kubernetes_Deployment.md`.
