# Enterprise Digital Platform

Monorepo untuk Enterprise Digital Platform. Dokumentasi lengkap ada di [`Enterprise_Digital_Platform_Documentation/`](./Enterprise_Digital_Platform_Documentation), khususnya [`01_Vision_and_Roadmap.md`](./Enterprise_Digital_Platform_Documentation/01_Vision_and_Roadmap.md).

## Prinsip Platform
- Multi Company (`company_id` pada seluruh data transaksi)
- Multi Branch, Multi Departement
- Role Based Access Control (RBAC)
- Audit Log
- JWT/OAuth2
- Microservices, Event Driven (Kafka)
- PostgreSQL + ClickHouse + Redis + MinIO
- Frontend: React + Bootstrap 5
- Backend: Go

## Struktur Repo

```
Enterprise_Digital_Platform/
├── backend/                           # seluruh kode Go (BE)
│   ├── services/                      # core microservices
│   │   ├── api-gateway/               # single entry point untuk seluruh client
│   │   ├── auth-service/              # JWT/OAuth2, login, token
│   │   ├── company-service/           # Company, Branch, Department
│   │   ├── rbac-service/              # Role, Permission, penugasan role
│   │   └── audit-service/             # audit trail (konsumen event Kafka)
│   ├── modules/                       # placeholder modul bisnis (Fase 2)
│   │   ├── finance-service/  hr-service/  sales-service/  purchasing-service/
│   │   └── warehouse-service/  production-service/  qc-service/  asset-service/  ai-service/  bi-service/
│   └── go.work                        # Go workspace, menyatukan seluruh service Go
├── frontend/                          # seluruh kode client (FE)
│   └── web/                           # React 18 + Bootstrap 5 (Vite)
├── infra/                             # docker-compose (Postgres, ClickHouse, Redis, MinIO, Kafka), K8s placeholder
└── Enterprise_Digital_Platform_Documentation/   # dokumen arsitektur & roadmap
```

Backend dan frontend dipisah sebagai folder mandiri di root repo (`backend/` vs `frontend/`), sementara tetap dalam satu repo Git. Setiap service Go di `backend/services/` adalah Go module independen (punya `go.mod` sendiri) yang disatukan lewat `backend/go.work` — memudahkan development lintas service tanpa kehilangan batas modul yang jelas untuk kelak dipisah menjadi image/deployment sendiri-sendiri.

## Menjalankan secara lokal

1. Nyalakan infrastruktur:
   - **Postgres**: jalan native (bukan Docker) di mesin dev ini — lihat setup di `infra/README.md`.
   - **ClickHouse, Redis, MinIO, Kafka**:
     ```
     cd infra
     cp .env.example .env
     ./scripts/dev-up.ps1
     ```
2. Jalankan sebuah service (contoh auth-service):
   ```
   cd backend/services/auth-service
   go run ./cmd/server
   ```
3. Jalankan frontend:
   ```
   cd frontend/web
   npm install
   npm run dev
   ```

## User Role
| Role | Hak Akses |
|---|---|
| Super Admin | Semua Company |
| Company Admin | Company sendiri |
| Branch Manager | Branch sendiri |
| Finance | Finance |
| HR | HRIS |
| Sales | Sales |
| Purchasing | Purchasing |
| Warehouse | Gudang |
| Production | MES |
| QC | Quality |
| Asset | Asset |
| Auditor | Read Only |
| AI Analyst | AI & BI |

## Roadmap
- **Fase 1 ✅ selesai**: `backend/services/{api-gateway,auth-service,company-service,rbac-service,audit-service}` — login JWT, CRUD company/branch/department, role & permission management, audit trail via Kafka, semua sudah fungsional (bukan skeleton lagi).
- **Fase 2 (sedang berjalan)**: implementasi modul bisnis di `backend/modules/`.
  - ✅ **Finance** — Chart of Accounts, General Ledger/Journal, Invoices (AR/AP) dengan auto-posting ke GL, AR/AP summary.
  - ⏳ HR, Sales, Purchasing, Warehouse, Production, QC, Asset, AI, BI — belum dikerjakan.
  - Mengikuti dokumen `04_Database_Design.md`, `09_Kafka_Streaming.md`, `20_Implementation_Guide.md`. Lihat [`NEXT_SESSION.md`](./NEXT_SESSION.md) untuk status detail & panduan lanjutan.
- **Fase 3**: data warehouse/lake, big data pipeline, dashboard BI, deployment Kubernetes, monitoring, disaster recovery — lihat dokumen `05`–`16`.
