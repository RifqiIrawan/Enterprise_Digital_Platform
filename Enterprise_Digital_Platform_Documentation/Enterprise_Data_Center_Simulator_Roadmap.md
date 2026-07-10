# Enterprise Data Center Simulator (EDCS) Roadmap

> Versi 1.0

## Visi
Membangun platform simulasi enterprise yang menghasilkan data operasional perusahaan secara real-time untuk kebutuhan Big Data, Data Warehouse, BI, AI, dan DevOps.

## Target
- Simulator ERP
- Simulator HRIS
- Simulator Manufacturing (MES)
- Simulator Warehouse
- Simulator IoT
- Data Lake
- Data Warehouse
- Dashboard BI
- Machine Learning
- AI Chatbot
- CI/CD
- Kubernetes

## Arsitektur

```text
ERP / HRIS / MES / WMS / IoT
            │
     Data Generator
            │
      Apache Kafka
            │
     ETL (NiFi/Airbyte)
            │
 Data Lake (MinIO)
            │
ClickHouse + PostgreSQL
            │
 Power BI / Superset
            │
 ML / AI Chatbot
```

# Roadmap

## Phase 0 - Foundation (2 Minggu)
- GitHub Repository
- Docker & Docker Compose
- Kubernetes (K3s)
- PostgreSQL
- ClickHouse
- Redis
- MinIO
- Grafana
- Prometheus

## Phase 1 - Core Platform (3 Minggu)
- Authentication
- RBAC
- Organization
- Company
- Branch
- Employee
- Audit Log
- Notification

## Phase 2 - ERP Simulator (4 Minggu)
- Customer
- Supplier
- Product
- Sales Order
- Purchase Order
- Inventory
- Warehouse
- Stock Transfer
- Invoice
- Payment

## Phase 3 - Manufacturing (4 Minggu)
- BOM
- Formula
- Production Order
- MES
- Machine
- Shift
- Downtime
- OEE
- Quality Control

## Phase 4 - HRIS (3 Minggu)
- Attendance
- Leave
- Overtime
- Payroll Simulator
- KPI

## Phase 5 - Asset (3 Minggu)
- Asset Register
- Maintenance
- Calibration
- Depreciation

## Phase 6 - IoT Simulator (4 Minggu)
- MQTT
- Temperature
- Humidity
- Vibration
- RFID
- GPS
- Barcode

## Phase 7 - Data Engineering (4 Minggu)
- Apache Kafka
- Apache NiFi
- Airbyte
- ETL
- ELT
- Streaming
- Data Catalog

## Phase 8 - Big Data (3 Minggu)
- ClickHouse
- Partition
- Compression
- Materialized View

## Phase 9 - Business Intelligence (3 Minggu)
- CEO Dashboard
- Sales Dashboard
- Finance Dashboard
- Warehouse Dashboard
- Manufacturing Dashboard
- HR Dashboard

## Phase 10 - AI (5 Minggu)
- Forecasting
- Predictive Maintenance
- Anomaly Detection
- Recommendation Engine
- RAG Chatbot

## Phase 11 - Monitoring (2 Minggu)
- Grafana
- Prometheus
- Loki
- Alertmanager

## Phase 12 - DevOps (3 Minggu)
- GitHub Actions
- Jenkins
- Helm
- ArgoCD
- Backup
- Disaster Recovery

# Dataset Target

| Modul | Target Record |
|-------|--------------:|
| Customer | 500.000 |
| Supplier | 100.000 |
| Product | 100.000 |
| Sales | 5.000.000 |
| Purchase | 2.000.000 |
| Inventory | 10.000.000 |
| HRIS | 2.000.000 |
| Manufacturing | 15.000.000 |
| IoT | 50.000.000 |
| System Logs | 100.000.000 |

Generator akan mendukung skala 1 juta hingga 100 juta+ record melalui konfigurasi.

# Tech Stack
- Backend: Golang, .NET, Python
- Frontend: React + Vite
- Database: PostgreSQL, ClickHouse, Redis
- Object Storage: MinIO
- Streaming: Kafka
- ETL: Apache NiFi, Airbyte
- Monitoring: Grafana, Prometheus, Loki
- AI: Ollama, LangChain, MLflow
- Deployment: Docker, Kubernetes, GitHub Actions

# Deliverables
1. Enterprise Architecture
2. Microservices
3. Dataset Generator
4. ETL Pipeline
5. Dashboard BI
6. AI Module
7. CI/CD
8. Monitoring
9. Documentation
