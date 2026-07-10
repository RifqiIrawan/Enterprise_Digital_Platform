# 01 — Vision & Roadmap
## Enterprise Data Center Simulator (EDCS)

---

## 🎯 Visi

Membangun **platform enterprise simulator** kelas dunia yang mereplikasi ekosistem data center nyata secara end-to-end — mulai dari proses bisnis operasional (ERP, HRIS, CRM, WMS, MES) hingga infrastruktur data modern (Data Lake, Data Warehouse, Big Data, IoT) dan lapisan kecerdasan (AI/ML), semua terintegrasi dalam satu platform yang dapat digunakan untuk **pembelajaran, simulasi, dan demonstrasi**.

---

## 🏆 Misi

> "Memberikan pengalaman nyata ekosistem enterprise kepada setiap tim — dari developer, data engineer, data scientist, hingga business analyst — dalam satu lingkungan yang terkontrol, dapat diulang, dan mudah diobservasi."

---

## 🌐 Cakupan Platform

| Domain | Modul | Deskripsi Singkat |
|--------|-------|-------------------|
| **ERP** | Enterprise Resource Planning | Backbone integrasi seluruh modul bisnis |
| **HRIS** | Human Resource IS | Rekrutmen, payroll, absensi, performance |
| **CRM** | Customer Relationship Mgmt | Lead, pipeline, support, marketing |
| **WMS** | Warehouse Management | Inventory, picking, packing, shipping |
| **MES** | Manufacturing Execution | Produksi, BOM, OEE, quality |
| **Finance** | Financial Management | AP, AR, GL, fixed assets, tax |
| **Procurement** | Pengadaan | PR, RFQ, PO, vendor management |
| **Sales** | Sales Management | Order, pricing, commission, forecast |
| **Asset Mgmt** | Manajemen Aset | Lifecycle, maintenance, depreciation |
| **IoT** | Internet of Things | Device, sensor, real-time stream |
| **AI/ML** | Artificial Intelligence | Prediction, automation, NLP |
| **Big Data** | Pemrosesan Data Besar | Spark, distributed computing |
| **Data Warehouse** | OLAP & Dimensional | Star schema, dbt, OLAP cubes |
| **Data Lake** | Raw Data Storage | Medallion architecture, Delta Lake |
| **Dashboard BI** | Business Intelligence | Self-service, embedded analytics |
| **DevOps** | Platform Engineering | CI/CD, observability, security |

---

## 📅 Roadmap Master (21 Bulan)

### Phase 0 — Foundation (Bulan 1)
- [ ] Setup monorepo & arsitektur microservices
- [ ] Auth service (Keycloak), RBAC/ABAC
- [ ] API Gateway, Event Bus (Kafka)
- [ ] CI/CD pipeline dasar
- [ ] Infrastructure as Code (Terraform + Helm)

### Phase 1 — Core Business Modules (Bulan 2–5)
- [ ] ERP Core Engine + Master Data
- [ ] HRIS (payroll, absensi, rekrutmen)
- [ ] CRM (pipeline, ticketing, kampanye)
- [ ] Sales Module

### Phase 2 — Supply Chain & Operations (Bulan 5–9)
- [ ] WMS (inbound, inventory, outbound)
- [ ] MES (BOM, produksi, quality, OEE)
- [ ] Asset Management (lifecycle, CMMS)

### Phase 3 — Finance & Procurement (Bulan 8–11)
- [ ] Finance (AP, AR, GL, FP&A)
- [ ] Procurement (P2P full cycle)

### Phase 4 — Data Platform (Bulan 11–16)
- [ ] IoT Platform + Virtual Simulator
- [ ] Data Lake (Medallion Architecture)
- [ ] Data Warehouse (Kimball + dbt)
- [ ] ETL/ELT Pipelines (Airflow)

### Phase 5 — Intelligence (Bulan 15–19)
- [ ] ML Platform (MLflow, Feature Store)
- [ ] 8 Pre-built ML Use Cases
- [ ] AI Features (chatbot, OCR, RAG)

### Phase 6 — DevOps & Observability (Bulan 18–21)
- [ ] Full CI/CD (ArgoCD, GitHub Actions)
- [ ] Observability (Prometheus, ELK, Jaeger)
- [ ] Chaos Engineering
- [ ] Security (DevSecOps)

### Phase 7 — BI & Analytics (Bulan 19–21)
- [ ] 50+ Pre-built Dashboards
- [ ] Executive C-Level Dashboard
- [ ] Self-service BI
- [ ] Embedded Analytics per modul

---

## 📊 KPI Platform

| Metrik | Target |
|--------|--------|
| Jumlah modul terintegrasi | 16 modul |
| Jumlah microservices | 40–60 services |
| API endpoints terdokumentasi | 500+ |
| Virtual IoT device simulasi | 1.000+ |
| ML model deployed | 8 model |
| Pre-built dashboard | 50+ |
| Synthetic data volume | 10 GB+ |
| Service uptime | 99.9% |
| CI/CD deployment time | < 15 menit |

---

## 👥 Target Pengguna

| Persona | Kebutuhan |
|---------|-----------|
| **Data Engineer** | Pipeline, lake, warehouse, streaming |
| **Data Scientist** | ML platform, feature store, model serving |
| **Business Analyst** | BI dashboard, self-service analytics |
| **Developer** | API, microservices, DevOps tools |
| **Enterprise Architect** | Integration pattern, domain model |
| **IT Manager** | Observability, security, compliance |

---

## 🔑 Prinsip Desain

1. **Domain-Driven Design** — Bounded context per modul bisnis
2. **Event-Driven Architecture** — Kafka sebagai sistem saraf pusat
3. **API-First** — Semua fitur dapat dikonsumsi via REST/GraphQL
4. **Cloud-Native** — Container-first, berjalan di Kubernetes
5. **Observable by Default** — Metrics, logs, traces terintegrasi
6. **Security by Design** — Zero-trust, least privilege, audit trail
7. **Data Mesh** — Setiap domain memiliki ownership data sendiri
