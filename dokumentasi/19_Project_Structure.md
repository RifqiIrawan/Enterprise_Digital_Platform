# 19 — Project Structure
## Enterprise Data Center Simulator (EDCS)

---

## 🗂️ Monorepo Overview

EDCS menggunakan **monorepo** dengan **Nx** sebagai build system. Semua services, apps, dan packages berada dalam satu repository untuk memudahkan code sharing, atomic commits, dan dependency management.

```
enterprise-data-center-simulator/
├── .github/
│   ├── workflows/              # CI/CD pipelines
│   │   ├── ci.yml
│   │   ├── release.yml
│   │   ├── security-scan.yml
│   │   └── dr-test.yml
│   ├── CODEOWNERS
│   ├── pull_request_template.md
│   └── ISSUE_TEMPLATE/
│
├── apps/                       # Deployable applications
│   ├── web-portal/             # Main user-facing frontend
│   ├── admin-portal/           # Admin & DevOps portal
│   ├── mobile/                 # React Native (iOS + Android)
│   └── iot-simulator/          # Virtual IoT device simulator
│
├── services/                   # Backend microservices
│   ├── auth-service/
│   ├── erp-core-service/
│   ├── hris-service/
│   ├── payroll-service/
│   ├── crm-service/
│   ├── sales-service/
│   ├── wms-service/
│   ├── inventory-service/
│   ├── mes-service/
│   ├── quality-service/
│   ├── finance-service/
│   ├── accounting-service/
│   ├── procurement-service/
│   ├── vendor-service/
│   ├── asset-service/
│   ├── iot-gateway/
│   ├── notification-service/
│   ├── file-service/
│   ├── report-service/
│   ├── audit-service/
│   └── search-service/
│
├── data-platform/              # Data engineering
│   ├── spark-jobs/
│   │   ├── bronze/
│   │   ├── silver/
│   │   └── gold/
│   ├── kafka-connect/
│   │   └── connectors/
│   ├── airflow/
│   │   ├── dags/
│   │   ├── plugins/
│   │   └── config/
│   ├── dbt/
│   │   ├── models/
│   │   ├── tests/
│   │   ├── seeds/
│   │   ├── macros/
│   │   └── snapshots/
│   └── dataset-generator/
│
├── ml-platform/                # ML & AI
│   ├── notebooks/              # JupyterHub notebooks
│   │   ├── demand-forecast/
│   │   ├── predictive-maintenance/
│   │   ├── churn-prediction/
│   │   └── anomaly-detection/
│   ├── training/               # Training pipelines
│   ├── serving/                # BentoML services
│   ├── feature-store/          # Feast feature definitions
│   └── ai-assistant/           # RAG + LLM integration
│
├── infra/                      # Infrastructure as Code
│   ├── terraform/
│   │   ├── modules/
│   │   │   ├── kubernetes/
│   │   │   ├── networking/
│   │   │   ├── storage/
│   │   │   └── monitoring/
│   │   ├── environments/
│   │   │   ├── local/
│   │   │   ├── staging/
│   │   │   └── production/
│   │   └── main.tf
│   ├── kubernetes/             # Raw K8s manifests
│   │   ├── namespaces/
│   │   ├── rbac/
│   │   ├── network-policies/
│   │   └── storage-classes/
│   ├── helm/                   # Helm charts
│   │   ├── charts/             # Service charts
│   │   │   └── {service-name}/
│   │   └── platform/           # Platform charts
│   │       ├── kafka/
│   │       ├── postgresql/
│   │       ├── redis/
│   │       ├── minio/
│   │       ├── keycloak/
│   │       ├── prometheus-stack/
│   │       └── elk-stack/
│   └── docker/
│       ├── base-images/
│       └── docker-compose/
│           ├── docker-compose.yml          # Full stack
│           ├── docker-compose.dev.yml      # Dev override
│           └── docker-compose.data.yml     # Data platform only
│
├── packages/                   # Shared libraries
│   ├── shared-types/           # TypeScript interfaces
│   ├── shared-utils/           # Common utilities
│   ├── event-schemas/          # Kafka event schemas (Avro/JSON)
│   ├── api-client/             # Generated API client (OpenAPI)
│   ├── ui-components/          # Shared React components
│   ├── auth-middleware/        # JWT validation middleware
│   ├── db-utils/               # Database helpers
│   ├── kafka-utils/            # Kafka producer/consumer helpers
│   └── logger/                 # Structured logging
│
├── bi/                         # Business Intelligence
│   ├── superset/
│   │   ├── dashboards/         # Dashboard export JSON
│   │   ├── datasets/           # Dataset definitions
│   │   └── charts/             # Chart configs
│   └── grafana/
│       └── dashboards/         # Grafana dashboard JSON
│
├── docs/                       # Documentation
│   ├── 01_Vision_and_Roadmap.md
│   ├── 02_Enterprise_Architecture.md
│   ├── ... (dokumen ini)
│   ├── api/                    # OpenAPI specs
│   │   └── {service}.openapi.yaml
│   ├── architecture/
│   │   ├── adr/                # Architecture Decision Records
│   │   └── diagrams/           # Draw.io / Mermaid diagrams
│   ├── runbooks/               # Operational runbooks
│   └── onboarding/             # Developer onboarding guide
│
├── tests/                      # Cross-service tests
│   ├── e2e/                    # End-to-end test suites
│   │   ├── p2p-cycle/          # PR → PO → GR → Invoice
│   │   ├── order-to-cash/      # Order → Ship → Invoice → Payment
│   │   └── hire-to-retire/     # Recruit → Onboard → Payroll → Offboard
│   ├── integration/            # Integration tests
│   ├── load/                   # Load testing (k6)
│   │   ├── scenarios/
│   │   └── reports/
│   └── chaos/                  # Chaos engineering scripts
│
├── scripts/                    # Development & ops scripts
│   ├── setup/
│   │   ├── install-tools.sh    # Install kubectl, helm, etc.
│   │   ├── setup-local.sh      # One-command local setup
│   │   └── seed-data.sh        # Seed synthetic data
│   ├── db/
│   │   ├── backup.sh
│   │   ├── restore.sh
│   │   └── migrate.sh
│   ├── deploy/
│   │   ├── deploy-service.sh
│   │   └── rollback.sh
│   └── dr/
│       ├── failover.sh
│       └── failback.sh
│
├── nx.json                     # Nx workspace config
├── package.json                # Root package.json
├── tsconfig.base.json          # Base TypeScript config
├── .eslintrc.json              # ESLint rules
├── .prettierrc                 # Prettier config
├── .editorconfig               # Editor config
├── .gitignore
├── .env.example                # Environment variables template
└── README.md
```

---

## 🏗️ Service Internal Structure

```
services/hris-service/
├── src/
│   ├── api/                    # HTTP layer
│   │   ├── v1/
│   │   │   ├── employees/
│   │   │   │   ├── employees.controller.ts
│   │   │   │   ├── employees.routes.ts
│   │   │   │   ├── employees.validator.ts
│   │   │   │   └── employees.dto.ts
│   │   │   ├── attendance/
│   │   │   ├── leaves/
│   │   │   └── payroll/
│   │   └── health.ts
│   │
│   ├── domain/                 # Business logic (pure)
│   │   ├── entities/
│   │   │   ├── Employee.ts
│   │   │   ├── Leave.ts
│   │   │   └── PayrollRun.ts
│   │   ├── use-cases/
│   │   │   ├── CreateEmployee.ts
│   │   │   ├── ProcessPayroll.ts
│   │   │   ├── ApproveLeave.ts
│   │   │   └── TerminateEmployee.ts
│   │   ├── repositories/       # Interfaces only
│   │   │   ├── IEmployeeRepository.ts
│   │   │   └── ILeaveRepository.ts
│   │   └── events/
│   │       ├── EmployeeCreatedEvent.ts
│   │       └── PayrollProcessedEvent.ts
│   │
│   ├── infrastructure/         # Adapters
│   │   ├── database/
│   │   │   ├── PostgreSQLEmployeeRepository.ts
│   │   │   ├── migrations/
│   │   │   └── seeds/
│   │   ├── messaging/
│   │   │   ├── KafkaEventPublisher.ts
│   │   │   └── consumers/
│   │   └── external/
│   │       ├── ERPServiceClient.ts     # Memanggil erp-core
│   │       └── NotificationClient.ts
│   │
│   ├── config/
│   │   ├── app.config.ts
│   │   ├── database.config.ts
│   │   └── kafka.config.ts
│   │
│   └── main.ts
│
├── test/
│   ├── unit/
│   │   ├── use-cases/
│   │   └── entities/
│   ├── integration/
│   │   └── api/
│   └── fixtures/
│
├── migrations/
│   ├── 001_create_employees.sql
│   ├── 002_create_departments.sql
│   └── 003_create_payroll_tables.sql
│
├── openapi.yaml                # API specification
├── Dockerfile
├── docker-compose.dev.yml
├── package.json
├── tsconfig.json
└── jest.config.ts
```

---

## 📦 Shared Packages Detail

### @edcs/shared-types
```typescript
// Semua TypeScript interfaces yang digunakan cross-service
export interface Employee { ... }
export interface SalesOrder { ... }
export interface KafkaBaseEvent<T> {
  event_id: string;
  event_type: string;
  occurred_at: string;
  source_service: string;
  payload: T;
}
```

### @edcs/event-schemas
```
event-schemas/
├── hris/
│   ├── employee-created.avsc
│   ├── payroll-processed.avsc
│   └── index.ts
├── sales/
│   ├── order-created.avsc
│   └── index.ts
└── iot/
    ├── sensor-reading.avsc
    └── index.ts
```

### @edcs/kafka-utils
```typescript
// Reusable Kafka producer & consumer
import { createProducer, createConsumer } from '@edcs/kafka-utils';

const producer = createProducer({ brokers: ['kafka:9092'] });
await producer.send('hris.employee.created', event);

const consumer = createConsumer({
  groupId: 'finance-hris-consumer',
  topics: ['hris.payroll.processed'],
  handler: async (event) => { ... }
});
```

---

## 🚀 Quick Start Commands

```bash
# 1. Clone & install
git clone https://github.com/edcs/enterprise-data-center-simulator
cd enterprise-data-center-simulator
npm install

# 2. Setup environment
cp .env.example .env
# Edit .env sesuai kebutuhan lokal

# 3. Start infrastructure (Kafka, PostgreSQL, Redis)
docker-compose -f infra/docker/docker-compose/docker-compose.yml up -d

# 4. Run migrations
npm run db:migrate:all

# 5. Seed data
npm run seed:all -- --config config/small.yaml

# 6. Start semua services (development mode)
nx run-many --target=serve --all --parallel=10

# 7. Start frontend
nx serve web-portal

# Akses:
# Web Portal:    http://localhost:3000
# API Gateway:   http://localhost:8000
# Kafka UI:      http://localhost:8080
# Grafana:       http://localhost:3001
# Superset:      http://localhost:8088
# MLflow:        http://localhost:5000
```

---

## 📋 Nx Task Registry

| Task | Command | Runs On |
|------|---------|---------|
| Build all | `nx run-many --target=build --all` | All projects |
| Test all | `nx run-many --target=test --all` | All projects |
| Lint all | `nx run-many --target=lint --all` | All projects |
| Build affected | `nx affected --target=build` | Changed + dependents |
| Dep graph | `nx graph` | Browser |
| Serve HRIS | `nx serve hris-service` | hris-service only |
| E2E tests | `nx e2e p2p-cycle-e2e` | e2e suite |
