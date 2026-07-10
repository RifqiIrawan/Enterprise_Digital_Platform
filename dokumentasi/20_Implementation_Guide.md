# 20 — Implementation Guide
## Enterprise Data Center Simulator (EDCS)

---

## 🚀 Getting Started

Panduan ini memandu tim dari nol hingga platform EDCS berjalan penuh. Dibagi menjadi **Sprint-based implementation plan** dengan estimasi effort per komponen.

---

## 👥 Team Structure (Recommended)

| Role | Jumlah | Responsibility |
|------|--------|----------------|
| Tech Lead / Architect | 1 | Desain arsitektur, code review, ADR |
| Backend Developer (Node.js) | 3 | Business microservices |
| Backend Developer (Python) | 2 | Data platform, ML |
| Frontend Developer | 2 | Web portal, dashboard |
| DevOps / Platform Engineer | 2 | K8s, CI/CD, monitoring |
| Data Engineer | 2 | Pipelines, DWH, dbt |
| ML Engineer | 1 | Model training, serving |
| QA Engineer | 1 | Test automation, E2E |
| **Total** | **14** | |

---

## 📅 Implementation Sprints (84 Minggu)

### 🟦 SPRINT 0 — Foundations (Minggu 1–4)

#### Week 1: Repo & CI/CD
```bash
# Day 1-2: Setup monorepo
npm install -g nx
npx create-nx-workspace@latest edcs \
  --preset=ts \
  --name=enterprise-data-center-simulator

# Add plugins
npm install -D @nx/node @nx/react @nx/docker @nx/python

# Day 3-4: CI/CD Pipeline
# Setup GitHub Actions (CI: lint → test → build → scan → push)
# Setup ArgoCD di K3s lokal

# Day 5: Developer onboarding
# Write CONTRIBUTING.md
# Setup pre-commit hooks (husky)
# Write local setup script
```

**Checklist Sprint 0:**
- [ ] Monorepo berjalan di lokal semua developer
- [ ] GitHub Actions CI pipeline hijau untuk dummy service
- [ ] ArgoCD terinstall dan bisa sync
- [ ] Code standards dokumen selesai
- [ ] Pre-commit hooks aktif

#### Week 2: Platform Services
```bash
# Auth Service (Keycloak)
helm install keycloak bitnami/keycloak \
  -n edcs-system \
  --set auth.adminUser=admin \
  --set auth.adminPassword=admin123

# Setup realm EDCS
# Import realm config dari infra/keycloak/realm-export.json

# API Gateway (Kong)
helm install kong kong/kong \
  -n edcs-system \
  -f infra/helm/platform/kong/values.yaml
```

#### Week 3-4: Infrastructure
```bash
# PostgreSQL per service (via Helm)
for service in erp hris crm wms mes finance procurement asset; do
  helm install postgres-$service bitnami/postgresql \
    -n edcs-storage \
    -f infra/helm/platform/postgresql/values-$service.yaml
done

# Kafka cluster
helm install kafka bitnami/kafka \
  -n edcs-data \
  -f infra/helm/platform/kafka/values.yaml

# Redis
helm install redis bitnami/redis \
  -n edcs-system \
  -f infra/helm/platform/redis/values.yaml

# MinIO (Data Lake storage)
helm install minio bitnami/minio \
  -n edcs-data \
  -f infra/helm/platform/minio/values.yaml
```

---

### 🟩 SPRINT 1 — Core Business (Minggu 5–20)

#### ERP Core Service (Week 5-8)

**Step 1: Scaffold service**
```bash
nx generate @nx/node:application erp-core-service \
  --directory=services/erp-core-service \
  --framework=express
```

**Step 2: Database migrations**
```sql
-- migrations/001_business_units.sql
CREATE TABLE business_units (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(20) UNIQUE NOT NULL,
  name VARCHAR(100) NOT NULL,
  parent_id UUID REFERENCES business_units(id),
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Seed: Business Units
INSERT INTO business_units (code, name) VALUES
  ('HQ', 'Headquarters'),
  ('OPS', 'Operations'),
  ('FIN', 'Finance & Administration'),
  ('SCM', 'Supply Chain'),
  ('MFG', 'Manufacturing'),
  ('SALES', 'Sales & Marketing');
```

**Step 3: Implement CRUD + Events**
```typescript
// services/erp-core-service/src/domain/use-cases/CreateProduct.ts
export class CreateProductUseCase {
  constructor(
    private readonly productRepo: IProductRepository,
    private readonly eventPublisher: IEventPublisher
  ) {}

  async execute(input: CreateProductDTO): Promise<Product> {
    // Validate
    await this.validateUniqueCode(input.product_code);

    // Create
    const product = Product.create(input);
    await this.productRepo.save(product);

    // Publish event
    await this.eventPublisher.publish('erp.master-data.product.created', {
      event_id: uuidv4(),
      event_type: 'PRODUCT_CREATED',
      occurred_at: new Date().toISOString(),
      source_service: 'erp-core-service',
      payload: product.toJSON()
    });

    return product;
  }
}
```

**ERP Milestones:**
- [ ] Master data API (products, customers, vendors) live
- [ ] Chart of Accounts dapat dikelola
- [ ] Event publishing ke Kafka berjalan
- [ ] Postman collection tersedia

#### HRIS Service (Week 9-16)

**Implementation order:**
1. Department & Position management
2. Employee CRUD + Photo upload
3. Attendance logging (multiple sources)
4. Leave management + approval workflow
5. Salary structure setup
6. Payroll engine (kalkulasi garis besar)
7. Payslip PDF generation
8. Employee self-service portal

**Payroll Engine Implementation:**
```typescript
// services/payroll-service/src/domain/use-cases/ProcessPayroll.ts
export class ProcessPayrollUseCase {
  async execute(period: string): Promise<PayrollRun> {
    // 1. Get active employees
    const employees = await this.employeeRepo.findActive();

    // 2. Process each employee
    const details: PayrollDetail[] = [];
    for (const emp of employees) {
      const detail = await this.calculatePayroll(emp, period);
      details.push(detail);
    }

    // 3. Create payroll run record
    const run = PayrollRun.create({
      period,
      total_employees: details.length,
      total_gross: sum(details, 'gross_salary'),
      total_deduction: sum(details, 'total_deduction'),
      total_net: sum(details, 'net_salary'),
    });

    await this.payrollRunRepo.saveWithDetails(run, details);

    // 4. Post to Finance (GL entry)
    await this.financeClient.postJournalEntry({
      description: `Payroll ${period}`,
      lines: this.buildGLLines(details),
    });

    // 5. Publish event
    await this.eventPublisher.publish('hris.payroll.processed', {
      run_id: run.id,
      period,
      total_net: run.total_net
    });

    return run;
  }

  private async calculatePayroll(
    employee: Employee,
    period: string
  ): Promise<PayrollDetail> {
    const components = await this.salaryRepo.getComponents(employee.id);
    const attendance = await this.attendanceRepo.getMonthlySummary(
      employee.id, period
    );
    const overtime = await this.attendanceRepo.getOvertime(employee.id, period);

    // Basic salary pro-rata jika ada absent
    const workingDays = attendance.scheduled_days;
    const presentDays = attendance.present_days;
    const basicSalary = components.basic * (presentDays / workingDays);

    // Allowances (tetap, tidak pro-rata)
    const totalAllowance = sum(components.allowances, 'amount');

    // Gross salary
    const grossSalary = basicSalary + totalAllowance + overtime.amount;

    // BPJS calculation
    const bpjsKesEmp = Math.min(grossSalary * 0.01, 120000);
    const bpjsJhtEmp = grossSalary * 0.02;
    const bpjsJpEmp  = Math.min(grossSalary * 0.01, 93600);

    // PPh 21 (simplified — metode TER)
    const pph21 = this.calculatePPh21(grossSalary * 12, employee.ptkp_status) / 12;

    const totalDeduction = pph21 + bpjsKesEmp + bpjsJhtEmp + bpjsJpEmp;
    const netSalary = grossSalary - totalDeduction;

    return {
      employee_id: employee.id,
      basic_salary: basicSalary,
      total_allowance: totalAllowance,
      gross_salary: grossSalary,
      pph21,
      bpjs_kesehatan_emp: bpjsKesEmp,
      bpjs_tk_jht_emp: bpjsJhtEmp,
      bpjs_tk_jp_emp: bpjsJpEmp,
      total_deduction: totalDeduction,
      net_salary: netSalary,
      attendance_days: presentDays,
      overtime_hours: overtime.hours,
      overtime_amount: overtime.amount,
    };
  }
}
```

---

### 🟨 SPRINT 2 — Data Platform (Minggu 45–62)

#### Data Lake Setup (Week 45-52)
```bash
# 1. Setup Delta Lake di MinIO
pip install delta-spark pyspark

# 2. Jalankan setup script
python data-platform/setup/init_lake_structure.py

# 3. Deploy Spark cluster di K8s
helm install spark bitnami/spark \
  -n edcs-data \
  -f infra/helm/platform/spark/values.yaml

# 4. Setup Kafka Connect untuk CDC
kubectl apply -f infra/kubernetes/kafka-connect/debezium-connectors.yaml

# 5. Verify data flowing
python data-platform/spark-jobs/bronze/verify_ingestion.py
```

#### dbt Setup (Week 56-62)
```bash
# 1. Install dbt
pip install dbt-clickhouse

# 2. Init project
cd data-platform/dbt
dbt init edcs_dwh

# 3. Configure profiles
cat > profiles.yml << 'EOF'
edcs_dwh:
  target: dev
  outputs:
    dev:
      type: clickhouse
      host: clickhouse
      port: 9000
      user: dbt_user
      password: "{{ env_var('DBT_CLICKHOUSE_PASS') }}"
      database: edcs_dwh
      schema: mart
EOF

# 4. Run first transformation
dbt run --select staging.hris

# 5. Test data quality
dbt test --select staging.hris

# 6. Generate docs
dbt docs generate && dbt docs serve
```

---

### 🟧 SPRINT 3 — ML Platform (Minggu 63–74)

#### MLflow Setup
```bash
# Deploy MLflow
helm install mlflow community-charts/mlflow \
  -n edcs-ml \
  --set artifactRoot="s3://edcs-mlflow/artifacts" \
  --set backendStore.postgres.enabled=true

# Verify
mlflow server --host 0.0.0.0 &
curl http://localhost:5000/health
```

#### Train First Model (Demand Forecasting)
```python
# ml-platform/training/demand_forecast/train.py
import mlflow
import mlflow.sklearn
from xgboost import XGBRegressor
from sklearn.model_selection import TimeSeriesSplit

mlflow.set_tracking_uri("http://mlflow.edcs-ml:5000")
mlflow.set_experiment("demand-forecasting")

with mlflow.start_run(run_name="xgb_baseline_v1"):
    # Log parameters
    params = {"n_estimators": 500, "learning_rate": 0.05, "max_depth": 6}
    mlflow.log_params(params)

    # Train
    model = XGBRegressor(**params)
    model.fit(X_train, y_train)

    # Evaluate
    mape = mean_absolute_percentage_error(y_test, model.predict(X_test))
    mlflow.log_metric("mape", mape)

    # Log model
    mlflow.sklearn.log_model(
        model,
        "model",
        registered_model_name="demand-forecast"
    )

    print(f"MAPE: {mape:.4f}")
    # Target: MAPE < 0.15 (15%)
```

---

## 🧪 Testing Strategy

### Unit Tests (per service)
```typescript
// Setiap use case harus 100% unit tested
describe('ProcessPayroll', () => {
  it('should calculate basic salary pro-rata for absent days', async () => {
    const employee = createMockEmployee({ basic_salary: 10_000_000 });
    const attendance = { scheduled_days: 22, present_days: 20 };

    const result = await useCase.calculatePayroll(employee, attendance, '2026-06');

    expect(result.basic_salary).toBe(9_090_909.09); // 10M * (20/22)
  });

  it('should deduct PPh 21 correctly for K/0 status', async () => {
    // Test PPh 21 calculation
  });
});
```

### Integration Tests
```typescript
// Test full API endpoint dengan real database
describe('POST /v1/hris/employees', () => {
  it('should create employee and publish kafka event', async () => {
    const response = await request(app)
      .post('/v1/hris/employees')
      .set('Authorization', `Bearer ${token}`)
      .send(validEmployeePayload);

    expect(response.status).toBe(201);
    expect(response.body.data.employee_code).toBeDefined();

    // Verify Kafka event published
    const kafkaMessage = await consumeKafkaMessage('hris.employee.created');
    expect(kafkaMessage.payload.employee_id).toBe(response.body.data.id);
  });
});
```

### E2E Tests (Business Scenarios)
```typescript
// tests/e2e/hire-to-retire/full-cycle.test.ts
describe('Hire-to-Retire E2E', () => {
  it('should complete full employee lifecycle', async () => {
    // 1. Create employee
    const employee = await hrisClient.createEmployee(employeeData);

    // 2. Run attendance for a month
    await seedAttendance(employee.id, '2026-07');

    // 3. Process payroll
    const payrollRun = await hrisClient.processPayroll('2026-07');

    // 4. Verify GL entry created in Finance
    const journalEntry = await financeClient.getJournalByRef(
      'PAYROLL', payrollRun.id
    );
    expect(journalEntry.status).toBe('POSTED');

    // 5. Terminate employee
    await hrisClient.terminateEmployee(employee.id, {
      termination_date: '2026-08-31',
      reason: 'RESIGNATION'
    });

    // 6. Verify access revoked
    await expect(
      authClient.login(employee.email, employee.temp_password)
    ).rejects.toThrow('Account disabled');
  });
});
```

---

## 📊 Definition of Done (per Feature)

```markdown
Sebuah fitur dianggap DONE ketika:
- [ ] Unit tests coverage > 80%
- [ ] Integration tests untuk semua API endpoints
- [ ] OpenAPI spec diupdate
- [ ] Kafka events terdokumentasi di event-schemas
- [ ] Database migrations tersedia
- [ ] Health check endpoint tidak berubah
- [ ] Prometheus metrics ter-expose
- [ ] Structured logging menggunakan format standard
- [ ] README service diupdate
- [ ] Code review oleh minimal 1 peer
- [ ] SonarQube Quality Gate: PASSED
- [ ] Tidak ada HIGH/CRITICAL vulnerability
- [ ] Deploy ke staging berhasil
- [ ] Smoke test di staging hijau
```

---

## 🔧 Common Development Commands

```bash
# Generate new service
nx generate @nx/node:application {service-name} --directory=services/{service-name}

# Run migration untuk service tertentu
npm run db:migrate -- --service=hris

# Regenerate API client dari OpenAPI spec
npm run generate:api-client -- --service=hris

# Tail logs service di K8s
kubectl logs -f -n edcs-business -l app=hris-service

# Port-forward service untuk debugging
kubectl port-forward -n edcs-business svc/hris-service 3003:3003

# Run load test
k6 run tests/load/scenarios/hris-load.js --vus 50 --duration 5m

# Check Kafka consumer lag
kubectl exec -n edcs-data kafka-0 -- \
  kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
  --group hris-consumer --describe
```
