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
| **Fase 7** | Data Warehouse — dw-service: 9 fact tables (ClickHouse), Batch ETL, Kafka Streaming ETL | ✅ Selesai |
| **Fase 8** | Data Lake — MinIO bronze layer (JSON Lines), dual-write dengan ClickHouse | ✅ Selesai |

---

## Apa yang Sudah Dibangun

### 16 Service Go (berjalan sekaligus)

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

### Integrasi Utama

- **Finance posting**: HR, Sales, Purchasing → Finance (via HTTP)
- **Stock movement**: Sales, Purchasing, Production → Warehouse (via HTTP)
- **Audit trail**: Semua service → Kafka → audit-service → Postgres
- **DW Batch ETL**: dw-service → 9 Postgres DB → ClickHouse + MinIO (setiap 5 menit)
- **DW Streaming ETL**: Kafka (12 topics) → dw-service → ClickHouse + MinIO (<100ms)
- **IoT Pipeline**: iot-service simulator → MQTT → Mosquitto → subscribe → Postgres + Kafka

### Frontend

React SPA dengan 40+ halaman, RBAC-driven sidebar, multi-tenant company/branch switcher, DataTable (search + sort + pagination) di semua halaman list.

---

## Apa yang Belum Ada

Ini adalah platform yang sudah berfungsi penuh, bukan "belum selesai". Yang berikut adalah fitur tambahan yang bisa dikerjakan kalau ada kebutuhan:

| Fitur | Deskripsi |
|-------|-----------|
| **Monitoring/Observability** | Prometheus metrics, Grafana dashboard, structured logging (JSON) |
| **ClickHouse Materialized View** | Pre-aggregation untuk query analitik yang lebih cepat |
| **Silver/Gold Data Lake** | Transformation layer di atas MinIO bronze (butuh Spark atau dbt) |
| **Modul bisnis tambahan** | CRM, Ticketing, E-Commerce, dll |
| **Frontend charts di BI** | Visualisasi data dari ClickHouse fact tables |
| **Production deployment** | Real cloud infra (managed Postgres, Kafka cluster, K8s managed) |

---

## Cara Menjalankan

Lihat `20_Implementation_Guide.md` untuk panduan lengkap.

**Quick start**:
```bash
# 1. Start infra
cd infra && docker compose up -d

# 2. Start semua 16 Go services (masing-masing di terminal sendiri)
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

CI/CD: GitHub Actions — backend matrix test (16 services) + frontend build, hijau di semua commit.
