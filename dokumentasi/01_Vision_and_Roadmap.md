# 01 — Vision & Roadmap
## Enterprise Digital Platform (EDP)

---

## Visi

Membangun **platform enterprise end-to-end yang sepenuhnya berfungsi** — bukan simulator atau mockup — yang mereplikasi ekosistem sistem informasi bisnis nyata: mulai dari transaksi operasional harian (Finance, HR, Sales, Purchasing, Warehouse, Production, QC, Asset) hingga lapisan data (IoT, Data Warehouse, Data Lake) dan analitik (BI Dashboard, Forecasting, Anomaly Detection).

Platform ini dirancang untuk **pembelajaran mendalam dan demonstrasi** ekosistem enterprise dengan implementasi yang bisa dijalankan dan diverifikasi langsung — semua service benar-benar jalan, semua data benar-benar tersimpan, semua integrasi benar-benar dieksekusi.

---

## Stack Teknologi Aktual

| Layer | Teknologi |
|-------|-----------|
| **Backend** | Go 1.25 (semua 16 service) |
| **Frontend** | React 18 + Vite |
| **Database** | PostgreSQL 18 (1 DB per service) |
| **Message Broker** | Apache Kafka (bitnamilegacy/kafka:3.7.1, KRaft mode) |
| **Cache** | Redis 7 |
| **MQTT** | Eclipse Mosquitto 2 |
| **OLAP / Data Warehouse** | ClickHouse 24.3 |
| **Object Storage / Data Lake** | MinIO |
| **Container Orchestration** | Kubernetes + Kustomize |
| **CI/CD** | GitHub Actions |

---

## Modul yang Sudah Dibangun

| Fase | Modul | Status |
|------|-------|--------|
| **Fase 1 — Platform** | Auth, Company/Branch, RBAC, Audit Trail, API Gateway | ✅ Selesai |
| **Fase 2 — Bisnis** | Finance (GL/Invoice), HR (Karyawan/Absensi/Payroll), Sales (Quotation/SO), Purchasing (PR/PO), Warehouse (Produk/Stok/Transfer/Opname), Production (BOM/Work Order), QC (Standar/Inspeksi), Asset (Aset/Maintenance) | ✅ Selesai |
| **Fase 3 — Analitik** | AI-BI Service (Dashboard, Forecasting, Anomaly Detection) | ✅ Selesai |
| **Fase 4 — Hardening** | Automated tests (251+ test, 1 bug produksi ditemukan), Branch-level filtering, Company/Branch switcher | ✅ Selesai |
| **Fase 5 — Production-Readiness** | Dockerfile + docker-compose, K8s Kustomize manifests, Env config templates, GitHub Actions CI | ✅ Selesai |
| **Fase 6 — IoT** | iot-service: Device registry, MQTT pipeline, threshold alerts, IoT Simulator | ✅ Selesai |
| **Fase 7/8 — Data Layer** | dw-service: 9 fact tables (ClickHouse), Batch ETL + Kafka Streaming ETL, Data Lake (MinIO JSON Lines) | ✅ Selesai |

---

## Prinsip Desain

- **Benar-benar berfungsi, bukan mock** — setiap fitur diverifikasi end-to-end (curl/Playwright/unit test), bukan hanya ditulis
- **Go polos** — tidak ada framework ORM/DI, cukup `net/http` + `pgx` + `kafka-go`; mudah dibaca dan dipahami
- **Satu database per service** — tidak ada shared database; dw-service yang membaca langsung ke Postgres service lain adalah pengecualian eksplisit untuk analytical extraction (read-only)
- **Best-effort untuk side-channel** — Kafka publish, MQTT, MinIO lake write semuanya tidak-blocking dan graceful degradation kalau infrastruktur tidak tersedia; service tetap jalan tanpa mereka
- **Verifikasi nyata** — setiap fitur baru diverifikasi dengan infrastruktur sungguhan yang jalan, bukan asumsi dari baca kode
