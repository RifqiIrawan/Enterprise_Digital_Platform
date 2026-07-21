# 03 — Microservices Architecture
## Enterprise Digital Platform (EDP)

---

## Service Catalog

Semua service ditulis dalam **Go 1.25**, dijalankan dengan `go run ./cmd/server` (dev) atau binary dari Dockerfile (Docker/K8s).

### Platform Services (`backend/services/`)

| Service | Port | Database | Fungsi |
|---------|------|----------|--------|
| `api-gateway` | 8079 | — (Redis untuk rate limit) | JWT validation, reverse proxy ke semua service |
| `auth-service` | 8081 | `auth_service` | Register, login, JWT issue, user management |
| `company-service` | 8082 | `company_service` | Multi-tenant company dan branch management |
| `rbac-service` | 8083 | `rbac_service` | Role, menu permission, RBAC seed |
| `audit-service` | 8084 | `audit_service` | Kafka consumer → audit_logs, ClickHouse pipeline |

### Business Module Services (`backend/modules/`)

| Service | Port | Database | Domain |
|---------|------|----------|--------|
| `finance-service` | 8085 | `finance_service` | Chart of Accounts, Journal Entry (GL), Invoice (AR/AP) |
| `hr-service` | 8086 | `hr_service` | Karyawan, Absensi, Payroll (posting ke GL) |
| `sales-service` | 8087 | `sales_service` | Customer, Quotation, Sales Order (invoice AR, stock out) |
| `purchasing-service` | 8088 | `purchasing_service` | Supplier, Requisition, Purchase Order (invoice AP, stock in) |
| `warehouse-service` | 8089 | `warehouse_service` | Produk, Gudang, Stock Movement/Balance, Transfer, Opname |
| `production-service` | 8090 | `production_service` | Bill of Material, Work Order (konsumsi komponen + produksi) |
| `qc-service` | 8091 | `qc_service` | Standar Mutu, Inspeksi (PASS/FAIL/PARTIAL otomatis) |
| `asset-service` | 8092 | `asset_service` | Aset, Maintenance Schedule (overdue indicator) |
| `ai-bi-service` | 8093 | — (no DB, agregasi HTTP) | BI Dashboards, Forecasting (linear), Anomaly Detection (z-score) |
| `iot-service` | 8094 | `iot_service` | Device, IoT Reading, Alert (threshold), Simulator via MQTT |
| `dw-service` | 8095 | — (baca dari 9 DB lain, tulis ClickHouse) | ETL → ClickHouse facts, MinIO lake |

---

## Struktur Setiap Service

```
{service}/
├── cmd/server/main.go         # Wiring: config → DB connect → migrate → handler → listen
├── internal/
│   ├── config/config.go       # getEnv(key, default) pattern untuk semua env var
│   ├── model/                 # Struct domain (bukan entity ORM)
│   ├── store/store.go         # pgxpool.Connect + Migrate (embed FS)
│   ├── eventbus/eventbus.go   # kafka-go Writer, best-effort (nil-safe)
│   └── httpapi/
│       ├── handler.go         # Handler struct + Register(mux)
│       └── {domain}.go        # Handler per resource, Go 1.22+ pattern routing
├── migrations/
│   ├── embed.go               # //go:embed *.sql
│   └── 001_init.sql           # Schema + seed (migrasi bertahap kalau ada perubahan)
└── go.mod
```

Semua module terdaftar di `backend/go.work` sebagai Go workspace.

---

## Pola Konsisten di Semua Service

### Migrasi DB
`store.Migrate` dipanggil otomatis saat startup. Menggunakan tabel `schema_migrations` untuk idempotency — aman dijalankan ulang berkali-kali.

### Audit Event
Setiap operasi bisnis yang bermakna mempublikasikan `auditEvent` ke Kafka:
```go
type auditEvent struct {
    EventID       string    `json:"event_id"`
    EventType     string    `json:"event_type"`
    SourceService string    `json:"source_service"`
    OccurredAt    time.Time `json:"occurred_at"`
    ActorUserID   *string   `json:"actor_user_id,omitempty"`
    CompanyID     *string   `json:"company_id,omitempty"`
    Action        string    `json:"action"`
    EntityType    string    `json:"entity_type"`
    EntityID      string    `json:"entity_id"`
    Payload       any       `json:"payload,omitempty"`
}
```

### Branch-Level Filtering
Semua endpoint list mendukung `?branch_id=<uuid>` dengan filter NULL-inclusive:
```sql
WHERE (branch_id = $N OR branch_id IS NULL)
```
Data lama tanpa branch tetap muncul di semua filter; hanya data yang di-tag branch tertentu yang disembunyikan kalau branch lain dipilih.

### Cross-Service HTTP Calls
Pattern yang dipakai untuk operasi lintas service (bukan lewat api-gateway):
- `internal/financeclient` — POST ke finance-service untuk buat + post journal entry
- `internal/warehouseclient` — POST ke warehouse-service untuk batch stock movement

Urutan: panggil service lain dulu, baru update status lokal setelah sukses (tidak ada distributed transaction).
