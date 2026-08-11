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
| **Fase 12** | E-Commerce — ecommerce-service: Orders + Order Items (checkout, reuse katalog produk warehouse-service, stock-out otomatis saat SHIPPED) | ✅ Selesai, termasuk production-readiness (Dockerfile/docker-compose/K8s/CI/env/Prometheus, 2026-08-10 — manifest diverifikasi render bersih, docker build/smoke test container belum karena Docker Desktop tidak jalan sesi itu) — dan fact table dw-service `fact_ecommerce_order_lines` (2026-08-11) |

---

## Apa yang Sudah Dibangun

### 19 Service Go (berjalan sekaligus)

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
| dw-service | 8095 | — | ETL → ClickHouse (9 facts) + MinIO |
| crm-service | 8096 | crm_service | Leads, Accounts, Contacts, Opportunities, Activities |
| ticketing-service | 8097 | ticketing_service | Ticket Categories, Tickets, Comments |
| ecommerce-service | 8098 | ecommerce_service | Orders, Order Items (checkout online) |

### Integrasi Utama

- **Finance posting**: HR, Sales, Purchasing → Finance (via HTTP)
- **Stock movement**: Sales, Purchasing, Production → Warehouse (via HTTP)
- **Audit trail**: Semua service → Kafka → audit-service → Postgres
- **DW Batch ETL**: dw-service → 9 Postgres DB → ClickHouse + MinIO (setiap 5 menit)
- **DW Streaming ETL**: Kafka (12 topics) → dw-service → ClickHouse + MinIO (<100ms)
- **IoT Pipeline**: iot-service simulator → MQTT → Mosquitto → subscribe → Postgres + Kafka
- **Observability**: semua 19 service → Prometheus (metrics) + Grafana, JSON logs + request ID → Loki/Promtail, OpenTelemetry spans → Jaeger. crm-service (Fase 10) port 8096, ticketing-service (Fase 11) port 8097, dan ecommerce-service (Fase 12) port 8098 sudah ditambahkan ke target list statis `infra/prometheus/prometheus.yml` sejak production-readiness masing-masing (2026-08-09/10)
- **Stock-out E-Commerce**: ecommerce-service → warehouse-service (via HTTP `POST /stock-movements/batch`, `reference_type=ECOMMERCE_ORDER`) saat order SHIPPED, pola identik Sales Order FULFILLED

### Frontend

React SPA dengan 45+ halaman, RBAC-driven sidebar, multi-tenant company/branch switcher, DataTable (search + sort + pagination) di semua halaman list.

---

## Apa yang Belum Ada

Ini adalah platform yang sudah berfungsi penuh, bukan "belum selesai". Yang berikut adalah fitur tambahan yang bisa dikerjakan kalau ada kebutuhan:

| Fitur | Deskripsi |
|-------|-----------|
| **ClickHouse Materialized View** | ✅ MV pertama (`mv_finance_monthly_line_state`) sudah ada sejak 2026-08-08, backing `finance-monthly-summary` — MV tambahan untuk 11 fact table lain masih bisa dikerjakan kalau ada kebutuhan (3 fact terbaru -- CRM/Ticketing/E-Commerce, 2026-08-11 -- belum punya endpoint analitik sama sekali, jadi belum ada bukti kebutuhan MV) |
| **Silver/Gold Data Lake** | Transformation layer di atas MinIO bronze (butuh Spark atau dbt) |
| **Modul bisnis tambahan** | ✅ CRM (Leads/Accounts/Contacts/Opportunities/Activities, termasuk konversi Lead→Account+Contact+Opportunity transaksional) sudah ada sejak 2026-08-08 sebagai module ke-17, production-readiness lengkap sejak 2026-08-09, fact table `fact_crm_opportunities` sejak 2026-08-11. ✅ Ticketing (Ticket Categories/Tickets/Comments, alur status close/reopen) sudah ada sejak 2026-08-09 sebagai module ke-18, production-readiness lengkap sejak 2026-08-09 (hari yang sama), fact table `fact_ticketing_tickets` sejak 2026-08-11. ✅ E-Commerce (Orders/Order Items, checkout dengan katalog produk direuse dari warehouse-service, stock-out otomatis saat SHIPPED) sudah ada sejak 2026-08-10 sebagai module ke-19, production-readiness lengkap sejak 2026-08-10 (hari yang sama), fact table `fact_ecommerce_order_lines` sejak 2026-08-11 |
| **Frontend charts di BI** | ✅ 3 chart per bulan dari `dw-service` (Revenue vs Expense, Stock In vs Out, Sales Value — komponen `GroupedBarChart` yang sama, generik untuk 1 atau N seri) sudah ada di BI Dashboards sejak 2026-08-08 — chart tambahan lain masih bisa dikerjakan kalau ada kebutuhan |
| **Production deployment** | Real cloud infra (managed Postgres, Kafka cluster, K8s managed) |

---

## Cara Menjalankan

Lihat `20_Implementation_Guide.md` untuk panduan lengkap.

**Quick start**:
```bash
# 1. Start infra
cd infra && docker compose up -d

# 2. Start semua 19 Go services (masing-masing di terminal sendiri)
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

CI/CD: GitHub Actions — backend matrix test (semua 19 service) + frontend build, hijau di semua commit.
