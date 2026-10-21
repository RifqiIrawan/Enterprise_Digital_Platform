# 19 — Project Structure
## Enterprise Digital Platform (EDP)

---

## Repository Overview

```
Enterprise_Digital_Platform/
├── .github/
│   └── workflows/
│       ├── backend-ci.yml      # Build/vet/test matrix untuk 16 Go services
│       └── frontend-ci.yml     # Lint + build untuk React frontend
│
├── backend/
│   ├── go.work                 # Go workspace — semua module terdaftar di sini
│   ├── go.work.sum
│   │
│   ├── services/               # Platform services (Fase 1)
│   │   ├── api-gateway/        # Port 28079 — JWT validation + reverse proxy
│   │   ├── auth-service/       # Port 28081 — Register, login, JWT issue
│   │   ├── company-service/    # Port 28082 — Multi-tenant company/branch
│   │   ├── rbac-service/       # Port 28083 — Role, menu, permission
│   │   └── audit-service/      # Port 28084 — Kafka consumer → audit_logs
│   │
│   └── modules/                # Business module services (Fase 2+)
│       ├── finance-service/    # Port 28085 — GL, Invoice
│       ├── hr-service/         # Port 28086 — Karyawan, Payroll
│       ├── sales-service/      # Port 28087 — Quotation, Sales Order
│       ├── purchasing-service/ # Port 28088 — PR, Purchase Order
│       ├── warehouse-service/  # Port 28089 — Produk, Stok
│       ├── production-service/ # Port 28090 — BOM, Work Order
│       ├── qc-service/         # Port 28091 — Inspeksi Kualitas
│       ├── asset-service/      # Port 28092 — Aset, Maintenance
│       ├── ai-bi-service/      # Port 28093 — BI, Forecasting, Anomaly
│       ├── iot-service/        # Port 28094 — Device, MQTT, Simulator
│       └── dw-service/         # Port 28095 — ETL → ClickHouse + MinIO
│
├── frontend/
│   └── web/                    # React 18 + Vite SPA
│       ├── src/
│       │   ├── components/
│       │   │   └── common/
│       │   │       ├── DataTable.jsx       # Search + sort + pagination
│       │   │       ├── Modal.jsx
│       │   │       └── ...
│       │   ├── context/
│       │   │   └── CompanyContext.jsx      # Multi-tenant state (company + branch)
│       │   ├── pages/
│       │   │   ├── auth/, admin/, finance/, hr/
│       │   │   ├── sales/, purchasing/, warehouse/
│       │   │   ├── production/, qc/, asset/
│       │   │   ├── aibi/, iot/, dw/
│       │   │   └── dashboard/
│       │   └── utils/
│       │       └── apiClient.js            # Axios instance dengan base URL
│       ├── Dockerfile                      # Multi-stage: node build → nginx
│       └── nginx.conf                      # SPA fallback: try_files $uri /index.html
│
├── infra/
│   ├── docker-compose.yml      # Full stack: 4 infra + 14 Go services + 1 frontend (19 containers)
│   ├── README.md               # Panduan port mapping, database setup
│   ├── .env.example
│   │
│   ├── environments/
│   │   ├── staging/            # 15 file .env.example (1 per service)
│   │   └── production/         # 15 file .env.example (1 per service)
│   │
│   ├── kafka/
│   │   └── topics.md           # Konvensi penamaan topic
│   │
│   ├── mosquitto/
│   │   └── mosquitto.conf      # Allow anonymous (dev only)
│   │
│   └── kubernetes/
│       ├── base/               # 14 Deployment + 14 Service + 14 ConfigMap + 1 Ingress
│       └── overlays/
│           ├── dev/            # namespace=edp-dev, local images, host.docker.internal
│           ├── staging/        # namespace=edp-staging, replicas=2
│           └── prod/           # namespace=edp-prod, replicas=3
│
├── dokumentasi/                # Dokumen arsitektur ini (20 file .md)
│
├── NEXT_SESSION.md             # Handoff doc: status terkini + known issues + next steps
└── README.md                   # Cara menjalankan project
```

---

## Struktur Setiap Go Service

```
{service}/
├── cmd/server/main.go          # Entry point: config → DB → migrate → handler → listen
├── deployments/Dockerfile      # Multi-stage Go build
├── internal/
│   ├── config/config.go        # Semua env var via getEnv(key, default)
│   ├── model/                  # Struct domain
│   ├── store/store.go          # pgxpool.Connect + Migrate(embed FS)
│   ├── eventbus/eventbus.go    # kafka-go Writer, nil-safe, best-effort
│   └── httpapi/
│       ├── handler.go          # Handler struct + Register(mux *http.ServeMux)
│       └── {domain}.go         # Handler functions, pattern routing Go 1.22+
├── migrations/
│   ├── embed.go                # //go:embed *.sql
│   └── 001_init.sql            # Schema awal (file baru untuk setiap perubahan)
└── go.mod
```

**Pengecualian**:
- `ai-bi-service`: tidak ada `store/`, `eventbus/` (no DB, no Kafka publish)
- `dw-service`: tidak ada `store/` (no own DB), punya `internal/sourcedb/`, `internal/clickhouse/`, `internal/etl/`, `internal/streaming/`, `internal/datalake/`
- `iot-service`: punya `internal/ingest/` (MQTT subscriber) tambahan

---

## Go Workspace

`backend/go.work` mendaftarkan semua 16 module:

```
go 1.25.0

use (
    ./services/api-gateway
    ./services/auth-service
    ./services/company-service
    ./services/rbac-service
    ./services/audit-service
    ./modules/finance-service
    ./modules/hr-service
    ...
    ./modules/dw-service
)
```

**Penting**: `go build ./...` di root `backend/` tidak berfungsi (workspace mode tidak expand `./...` dari direktori yang bukan module). Selalu jalankan dari dalam folder module individual.
