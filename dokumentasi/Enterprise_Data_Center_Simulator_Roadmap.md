# Enterprise Digital Platform — Status & Roadmap

---

## Status Keseluruhan

**Semua fase utama telah selesai diimplementasikan dan diverifikasi.**

| Fase | Deskripsi | Status |
|------|-----------|--------|
| **Fase 1** | Platform Foundation — Auth, Company, RBAC, Audit Trail, API Gateway | ✅ Selesai |
| **Fase 2** | Business Modules — Finance, HR, Sales, Purchasing, Warehouse, Production, QC, Asset | ✅ Selesai |
| **Fase 3** | Analytics — AI-BI Service (Dashboard, Forecasting, Anomaly Detection) | ✅ Selesai |
| **Fase 4** | Hardening — 251+ automated tests, branch-level filtering, company switcher | ✅ Selesai |
| **Fase 5** | Production-Readiness — Dockerfile, docker-compose, K8s Kustomize, env templates, CI | ✅ Selesai |
| **Fase 6** | IoT — iot-service: Device, MQTT pipeline, threshold alerts, IoT Simulator | ✅ Selesai |
| **Fase 7** | Data Warehouse — dw-service: 12 fact tables (ClickHouse), Batch ETL, Kafka Streaming ETL | ✅ Selesai |
| **Fase 8** | Data Lake — MinIO bronze layer (JSON Lines), dual-write dengan ClickHouse | ✅ Selesai |
| **Fase 9** | Observability — Prometheus/Grafana (metrics), JSON logs + Loki/Promtail, OpenTelemetry + Jaeger (tracing) | ✅ Selesai |
| **Fase 10** | CRM — crm-service: Leads, Accounts, Contacts, Opportunities, Activities, konversi Lead transaksional | ✅ Selesai, termasuk production-readiness (Dockerfile/docker-compose/K8s/CI/env/Prometheus, 2026-08-09) — dan fact table dw-service `fact_crm_opportunities` (2026-08-11) |
| **Fase 11** | Ticketing — ticketing-service: Ticket Categories, Tickets, Comments (helpdesk/customer support) | ✅ Selesai, termasuk production-readiness (Dockerfile/docker-compose/K8s/CI/env/Prometheus, 2026-08-09) — dan fact table dw-service `fact_ticketing_tickets` (2026-08-11) |
| **Fase 12** | E-Commerce — ecommerce-service: Orders + Order Items (checkout, reuse katalog produk warehouse-service, stock-out otomatis saat SHIPPED) | ✅ Selesai, termasuk production-readiness (Dockerfile/docker-compose/K8s/CI/env/Prometheus, 2026-08-10 — manifest diverifikasi render bersih; `docker compose build` + smoke test container menyusul 2026-08-15 saat Docker Desktop hidup lagi) — dan fact table dw-service `fact_ecommerce_order_lines` (2026-08-11) |
| **Fase 13** | Fleet & Delivery — fleet-service: Kendaraan, Pengemudi, Surat Jalan; status armada digerakkan lifecycle surat jalan, integrasi dua arah ke ecommerce-service (snapshot order SHIPPED → surat jalan, penyelesaian surat jalan → order DELIVERED) | ✅ Selesai, termasuk production-readiness (Dockerfile/docker-compose/K8s/CI/env/Prometheus, 2026-08-13, diverifikasi penuh 2026-08-15 — manifest render bersih + `docker compose build` sungguhan + smoke test container terhadap Postgres native) — fact table dw-service belum |
| **Fase 14** | Project Management — project-service: Proyek, Tugas, Timesheet; anggaran vs realisasi, integrasi ke hr-service (validasi + snapshot karyawan, tarif per jam diturunkan dari gaji pokok) dan finance-service (timesheet APPROVED diposting kolektif ke GL sebagai satu jurnal) | ✅ Selesai (core, 2026-08-16) — production-readiness dan fact table dw-service belum |

---

## Apa yang Sudah Dibangun

### 21 Service Go (berjalan sekaligus)

| Service | Port | DB | Fitur Utama |
|---------|------|----|-------------|
| api-gateway | 8079 | — | JWT validation, reverse proxy |
| auth-service | 8081 | auth_service | Register, login, JWT |
| company-service | 8082 | company_service | Multi-tenant company/branch |
| rbac-service | 8083 | rbac_service | Role, menu, permission |
| audit-service | 8084 | audit_service | Kafka → audit_logs, ClickHouse |
| finance-service | 8085 | finance_service | GL, CoA, Invoice AR/AP |
| hr-service | 8086 | hr_service | Karyawan, Absensi, Payroll |
| sales-service | 8087 | sales_service | Customer, Quotation, SO |
| purchasing-service | 8088 | purchasing_service | Supplier, PR, PO |
| warehouse-service | 8089 | warehouse_service | Produk, Stok, Transfer, Opname |
| production-service | 8090 | production_service | BOM, Work Order |
| qc-service | 8091 | qc_service | Standar Mutu, Inspeksi |
| asset-service | 8092 | asset_service | Aset, Maintenance |
| ai-bi-service | 8093 | — | BI Dashboard, Forecasting, Anomaly |
| iot-service | 8094 | iot_service | Device, MQTT, Alert, Simulator |
| dw-service | 8095 | — | ETL → ClickHouse (12 facts) + MinIO |
| crm-service | 8096 | crm_service | Leads, Accounts, Contacts, Opportunities, Activities |
| ticketing-service | 8097 | ticketing_service | Ticket Categories, Tickets, Comments |
| ecommerce-service | 8098 | ecommerce_service | Orders, Order Items (checkout online) |
| fleet-service | 8099 | fleet_service | Kendaraan, Pengemudi, Surat Jalan |
| project-service | 8100 | project_service | Proyek, Tugas, Timesheet (biaya ke GL) |

### Integrasi Utama

- **Finance posting**: HR, Sales, Purchasing, Project → Finance (via HTTP)
- **Stock movement**: Sales, Purchasing, Production → Warehouse (via HTTP)
- **Audit trail**: Semua service → Kafka → audit-service → Postgres
- **DW Batch ETL**: dw-service → 9 Postgres DB → ClickHouse + MinIO (setiap 5 menit)
- **DW Streaming ETL**: Kafka (12 topics) → dw-service → ClickHouse + MinIO (<100ms)
- **IoT Pipeline**: iot-service simulator → MQTT → Mosquitto → subscribe → Postgres + Kafka
- **Observability**: 20 dari 21 service → Prometheus (metrics) + Grafana, JSON logs + request ID → Loki/Promtail, OpenTelemetry spans → Jaeger. project-service (Fase 14) SUDAH punya metrics/logging/tracing di kodenya tapi BELUM ada di target list Prometheus — itu bagian production-readiness yang sengaja ditunda. crm-service (Fase 10) port 8096, ticketing-service (Fase 11) port 8097, ecommerce-service (Fase 12) port 8098, dan fleet-service (Fase 13) port 8099 sudah ditambahkan ke target list statis `infra/prometheus/prometheus.yml` sejak production-readiness masing-masing (2026-08-09/10/13)
- **Stock-out E-Commerce**: ecommerce-service → warehouse-service (via HTTP `POST /stock-movements/batch`, `reference_type=ECOMMERCE_ORDER`) saat order SHIPPED, pola identik Sales Order FULFILLED
- **Penugasan orang**: project-service → hr-service (via HTTP `GET /employees/{id}`) untuk memvalidasi + men-snapshot nama karyawan pada proyek/tugas/timesheet, dan menurunkan tarif per jam default dari `basic_salary`

### Frontend

React SPA dengan 45+ halaman, RBAC-driven sidebar, multi-tenant company/branch switcher, DataTable (search + sort + pagination) di semua halaman list.

---

## Apa yang Belum Ada

Ini adalah platform yang sudah berfungsi penuh, bukan "belum selesai". Yang berikut adalah fitur tambahan yang bisa dikerjakan kalau ada kebutuhan:

| Fitur | Deskripsi |
|-------|-----------|
| **ClickHouse Materialized View** | ✅ MV pertama (`mv_finance_monthly_line_state`) sudah ada sejak 2026-08-08, backing `finance-monthly-summary` — MV tambahan untuk 11 fact table lain masih bisa dikerjakan kalau ada kebutuhan. `fact_crm_opportunities` sekarang punya endpoint analitik (`crm-pipeline-summary`, 2026-08-12) tapi sengaja masih query-only; `fact_ticketing_tickets`/`fact_ecommerce_order_lines` belum punya endpoint sama sekali |
| **Silver/Gold Data Lake** | Transformation layer di atas MinIO bronze (butuh Spark atau dbt) |
| **Modul bisnis tambahan** | ✅ CRM (Leads/Accounts/Contacts/Opportunities/Activities, termasuk konversi Lead→Account+Contact+Opportunity transaksional) sudah ada sejak 2026-08-08 sebagai module ke-17, production-readiness lengkap sejak 2026-08-09, fact table `fact_crm_opportunities` sejak 2026-08-11. ✅ Ticketing (Ticket Categories/Tickets/Comments, alur status close/reopen) sudah ada sejak 2026-08-09 sebagai module ke-18, production-readiness lengkap sejak 2026-08-09 (hari yang sama), fact table `fact_ticketing_tickets` sejak 2026-08-11. ✅ E-Commerce (Orders/Order Items, checkout dengan katalog produk direuse dari warehouse-service, stock-out otomatis saat SHIPPED) sudah ada sejak 2026-08-10 sebagai module ke-19, production-readiness lengkap sejak 2026-08-10 (hari yang sama), fact table `fact_ecommerce_order_lines` sejak 2026-08-11. ✅ Fleet & Delivery (Kendaraan/Pengemudi/Surat Jalan, integrasi dua arah ke ecommerce-service) sudah ada sejak 2026-08-12 sebagai module ke-20, production-readiness lengkap sejak 2026-08-13 — fact table dw-service SENGAJA belum dikerjakan. ✅ Project Management (Proyek/Tugas/Timesheet, integrasi ke hr-service + posting biaya kolektif ke GL finance-service) sudah ada sejak 2026-08-16 sebagai module ke-21 — production-readiness dan fact table SENGAJA belum dikerjakan (pola dua-tahap) |
| **Frontend charts di BI** | ✅ 4 chart dari `dw-service` di BI Dashboards — 3 time series bulanan (Revenue vs Expense, Stock In vs Out, Sales Value) sejak 2026-08-08, plus Pipeline CRM per Stage (2026-08-12) yang sumbu X-nya kategorikal, bukan waktu. Semuanya memakai komponen `GroupedBarChart` yang sama (generik untuk 1 atau N seri, sumbu kategori bisa di-override) — chart tambahan lain masih bisa dikerjakan kalau ada kebutuhan |
| **Production deployment** | Real cloud infra (managed Postgres, Kafka cluster, K8s managed) |

---

## Cara Menjalankan

Lihat `20_Implementation_Guide.md` untuk panduan lengkap.

**Quick start**:
```bash
# 1. Start infra
cd infra && docker compose up -d

# 2. Start semua 21 Go services (masing-masing di terminal sendiri)
cd backend/modules/finance-service && go run ./cmd/server
# ... (ulangi untuk semua service)

# 3. Start frontend
cd frontend/web && npm run dev

# 4. Buka http://localhost:3000
# Login: admin@edp.local / Admin@12345
```

---

## Repository

GitHub: [github.com/RifqiIrawan/Enterprise_Digital_Platform](https://github.com/RifqiIrawan/Enterprise_Digital_Platform) (public)

CI/CD: GitHub Actions — backend matrix test (20 dari 21 service; project-service menyusul saat production-readiness-nya dikerjakan) + frontend build, hijau di semua commit.
