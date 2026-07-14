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
│   ├── modules/                       # modul bisnis (Fase 2) + IoT Simulator (Fase 6)
│   │   ├── finance-service/  hr-service/  sales-service/  purchasing-service/
│   │   ├── warehouse-service/  production-service/  qc-service/  asset-service/
│   │   └── ai-bi-service/  iot-service/
│   └── go.work                        # Go workspace, menyatukan seluruh service Go
├── frontend/                          # seluruh kode client (FE)
│   └── web/                           # React 18 + Bootstrap 5 (Vite)
├── infra/                             # docker-compose (Postgres, ClickHouse, Redis, MinIO, Kafka, Mosquitto), K8s
└── Enterprise_Digital_Platform_Documentation/   # dokumen arsitektur & roadmap
```

Backend dan frontend dipisah sebagai folder mandiri di root repo (`backend/` vs `frontend/`), sementara tetap dalam satu repo Git. Setiap service Go di `backend/services/` adalah Go module independen (punya `go.mod` sendiri) yang disatukan lewat `backend/go.work` — memudahkan development lintas service tanpa kehilangan batas modul yang jelas untuk kelak dipisah menjadi image/deployment sendiri-sendiri.

## Menjalankan secara lokal

1. Nyalakan infrastruktur:
   - **Postgres**: jalan native (bukan Docker) di mesin dev ini — lihat setup di `infra/README.md`.
   - **ClickHouse, Redis, MinIO, Kafka, Mosquitto (MQTT, untuk IoT Simulator)**:
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

Alternatif: seluruh 15 service Go + frontend juga bisa dijalankan sekaligus sebagai
container (`cd infra && docker compose up -d --build`), tanpa perlu 16 terminal
manual — lihat "Full app stack di Docker" di `infra/README.md`. Postgres tetap
harus jalan native seperti langkah 1 di atas.

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
| IoT | IoT Simulator |

## Roadmap
- **Fase 1 ✅ selesai**: `backend/services/{api-gateway,auth-service,company-service,rbac-service,audit-service}` — login JWT, CRUD company/branch/department, role & permission management, audit trail via Kafka, semua sudah fungsional (bukan skeleton lagi).
- **Fase 2 ✅ selesai**: seluruh 9 modul bisnis di `backend/modules/` — Finance, HR, Sales, Purchasing, Warehouse, Production, QC, Asset, dan AI & BI (Dashboards, Forecasting, Anomaly Detection), semuanya fungsional & diverifikasi end-to-end. Production-readiness (Dockerfile, docker-compose, K8s manifests, environment config staging/prod, CI/CD GitHub Actions) juga sudah selesai. Lihat [`NEXT_SESSION.md`](./NEXT_SESSION.md) untuk status detail & panduan lanjutan.
- **Fase 6 (IoT Simulator) — 🚧 sebagian**: `backend/modules/iot-service` — device simulator (Temperature/Humidity/Vibration/RFID/GPS/Barcode) publish ke broker MQTT sungguhan (Mosquitto, `infra/mosquitto/`), di-ingest ke Postgres, alert ambang batas otomatis untuk device numerik, diverifikasi end-to-end lewat pipeline MQTT nyata. **Belum dikerjakan**: Dockerfile/K8s manifest/env config staging-prod/entry CI untuk iot-service (production-readiness khusus modul ini, ditunda sengaja — lihat catatan di `NEXT_SESSION.md`).
- **Fase 3, 4, 5, 7, 8, 9, 10, 11, 12** (data warehouse/lake, big data pipeline, HRIS lanjutan, asset lanjutan, data engineering, monitoring lanjutan, DevOps lanjutan sesuai penomoran roadmap asli): belum dikerjakan — lihat `Enterprise_Digital_Platform_Documentation/Enterprise_Data_Center_Simulator_Roadmap.md`.
