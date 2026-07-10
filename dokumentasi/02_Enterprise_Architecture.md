# 02 — Enterprise Architecture
## Enterprise Data Center Simulator (EDCS)

---

## 🏛️ Architectural Style

EDCS mengadopsi **Hybrid Architecture** yang menggabungkan:
- **Microservices** untuk domain bisnis yang independen
- **Event-Driven Architecture (EDA)** untuk komunikasi asinkron antar domain
- **CQRS + Event Sourcing** untuk modul dengan audit trail ketat (Finance, Procurement)
- **Data Mesh** untuk kepemilikan data per domain

---

## 🗺️ Landscape Arsitektur

```
┌─────────────────────────────────────────────────────────────────┐
│                        CLIENT LAYER                             │
│  Web Portal │ Admin Portal │ Mobile App │ BI Dashboard          │
└────────────────────────┬────────────────────────────────────────┘
                         │ HTTPS / WebSocket
┌────────────────────────▼────────────────────────────────────────┐
│                      API GATEWAY (Kong)                         │
│  Auth │ Rate Limiting │ Routing │ Load Balancing │ Caching      │
└────────────────────────┬────────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────────┐
│                   SERVICE MESH (Istio)                          │
│  mTLS │ Traffic Management │ Circuit Breaker │ Observability    │
└──┬──────┬──────┬──────┬──────┬──────┬──────┬──────┬────────────┘
   │      │      │      │      │      │      │      │
 ERP   HRIS   CRM   WMS   MES  FIN   PROC  SALES  ASSET  IOT
   │      │      │      │      │      │      │      │      │
└─────────────────────────────────────────────────────────────────┐
│                  EVENT BUS (Apache Kafka)                        │
│  Topics per Domain │ Schema Registry │ Dead Letter Queue        │
└─────────────────────────────────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────────┐
│                    DATA PLATFORM                                │
│  Data Lake │ Data Warehouse │ ML Platform │ BI Layer            │
└─────────────────────────────────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────────┐
│                 INFRASTRUCTURE LAYER                            │
│  Kubernetes │ Docker │ Terraform │ Vault │ Service Discovery    │
└─────────────────────────────────────────────────────────────────┘
```

---

## 🏗️ Architectural Decisions Record (ADR)

### ADR-001: Microservices over Monolith
**Status:** Accepted  
**Context:** Platform mencakup 16+ domain bisnis yang dikembangkan secara paralel  
**Decision:** Setiap domain menjadi microservice independen dengan database sendiri  
**Consequences:** Complexity meningkat, namun scalability & team autonomy optimal

### ADR-002: Kafka sebagai Event Backbone
**Status:** Accepted  
**Context:** Butuh komunikasi asinkron yang reliable antar 40+ services  
**Decision:** Apache Kafka sebagai event streaming platform  
**Consequences:** Eventual consistency, perlu Schema Registry (Avro/Protobuf)

### ADR-003: Kubernetes sebagai Runtime
**Status:** Accepted  
**Context:** Butuh orchestrasi container yang mature dan cloud-agnostic  
**Decision:** K3s untuk lokal dev, full K8s untuk staging/prod  
**Consequences:** Learning curve, namun portabilitas dan auto-healing terjamin

### ADR-004: PostgreSQL sebagai Primary Database
**Status:** Accepted  
**Context:** Modul bisnis butuh ACID compliance dan relational model  
**Decision:** PostgreSQL per service (database-per-service pattern)  
**Consequences:** Lebih banyak instance DB, namun isolasi domain terjaga

### ADR-005: Medallion Architecture untuk Data Lake
**Status:** Accepted  
**Context:** Data dari berbagai sumber perlu dikelola dengan kualitas bertingkat  
**Decision:** Bronze → Silver → Gold layers dengan Delta Lake format  
**Consequences:** Latency tambahan, namun data quality dan lineage terjaga

---

## 🔌 Integration Patterns

### 1. Synchronous (REST/gRPC)
Digunakan untuk: query real-time, user-facing operations
```
Service A ──REST──► Service B
          ◄─────────
```

### 2. Asynchronous (Event-Driven)
Digunakan untuk: proses bisnis yang tidak membutuhkan respons segera
```
Service A ──publish──► Kafka Topic ──consume──► Service B, C, D
```

### 3. Saga Pattern (untuk long-running transactions)
Digunakan untuk: P2P cycle, Order-to-Cash
```
Orchestrator ──► Service A ──► Service B ──► Service C
              ◄── (compensating transactions jika gagal)
```

### 4. CQRS
Digunakan untuk: Finance, Audit, Reporting
```
Write Model (Command) ──► Event Store ──► Read Model (Query)
```

---

## 🔒 Security Architecture

### Zero Trust Model
- **Never trust, always verify** — setiap request diautentikasi
- **Least privilege** — akses minimal yang dibutuhkan
- **Assume breach** — logging & monitoring menyeluruh

### Auth Flow
```
User ──► API Gateway ──► Keycloak (OAuth2/OIDC) ──► JWT Token
     ◄─── Token ◄───────────────────────────────────
User ──► Service (dengan JWT) ──► Verify ──► Response
```

### Secrets Management
- HashiCorp Vault untuk semua credentials
- Kubernetes Secrets (encrypted at rest)
- Auto-rotation credentials database

---

## 📦 Deployment Architecture

### Environment Strategy
| Environment | Tujuan | Infra |
|-------------|--------|-------|
| Local Dev | Development individual | Docker Compose |
| Integration | Testing integrasi | K3s (lokal) |
| Staging | Pre-production | K8s cloud (kecil) |
| Production | Live platform | K8s cloud (full) |

### Service Topology per Namespace
```
namespace: edcs-core
  ├── erp-service
  ├── auth-service
  ├── notification-service
  └── api-gateway

namespace: edcs-business
  ├── hris-service
  ├── crm-service
  ├── wms-service
  ├── mes-service
  ├── finance-service
  ├── procurement-service
  └── sales-service

namespace: edcs-data
  ├── kafka-cluster
  ├── data-lake-service
  ├── warehouse-service
  └── ml-platform

namespace: edcs-iot
  ├── mqtt-broker
  ├── iot-gateway
  └── device-simulator

namespace: edcs-observability
  ├── prometheus
  ├── grafana
  ├── elasticsearch
  ├── kibana
  └── jaeger
```

---

## 🗄️ Data Architecture Overview

| Layer | Teknologi | Tujuan |
|-------|-----------|--------|
| Operational DB | PostgreSQL | OLTP per service |
| Cache | Redis | Session, hot data |
| Document Store | MongoDB | Unstructured data |
| Message Queue | Kafka | Event streaming |
| Object Storage | MinIO | File, binary |
| Data Lake | Delta Lake / MinIO | Raw & curated data |
| Data Warehouse | ClickHouse | OLAP analytics |
| Vector DB | Qdrant | AI/ML embeddings |
| Search | Elasticsearch | Full-text & semantic |

---

## 📐 Non-Functional Requirements

| NFR | Target | Mekanisme |
|-----|--------|-----------|
| Availability | 99.9% | Multi-replica, health check |
| Latency (P95) | < 500ms | Caching, CDN, async |
| Throughput | 10.000 req/s | Horizontal scaling |
| Data Retention | 7 tahun | Tiered storage |
| RTO (Recovery Time) | < 1 jam | DR runbook otomatis |
| RPO (Recovery Point) | < 15 menit | Streaming replication |
| Security | OWASP Top 10 | DevSecOps pipeline |
