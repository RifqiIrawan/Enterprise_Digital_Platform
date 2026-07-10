# 17 — API Documentation
## Enterprise Data Center Simulator (EDCS)

---

## 📚 Overview

Semua EDCS services mengekspos **RESTful API** dengan standar **OpenAPI 3.1**. Dokumentasi dihasilkan otomatis menggunakan **Swagger UI** dan dipublikasikan di developer portal internal.

**Base URL:** `https://api.edcs.internal/v1`  
**Authentication:** Bearer JWT Token (via Keycloak)  
**Rate Limiting:** 1000 req/menit per API key  
**Versioning:** URI path (`/v1/`, `/v2/`)

---

## 🔐 Authentication

### Get Access Token
```http
POST https://auth.edcs.internal/realms/edcs/protocol/openid-connect/token
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
&client_id=my-app
&client_secret=my-secret
```

**Response:**
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 3600,
  "token_type": "Bearer",
  "scope": "hris:read finance:read"
}
```

### Use Token
```http
GET /v1/hris/employees
Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...
```

---

## 🏢 Standard Response Format

### Success Response
```json
{
  "success": true,
  "data": { ... },           // Single object atau array
  "meta": {                  // Untuk paginasi
    "page": 1,
    "per_page": 20,
    "total": 5243,
    "total_pages": 263
  },
  "timestamp": "2026-07-09T10:00:00Z"
}
```

### Error Response
```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": [
      { "field": "email", "message": "Invalid email format" },
      { "field": "hire_date", "message": "Date cannot be in the future" }
    ]
  },
  "timestamp": "2026-07-09T10:00:00Z",
  "request_id": "req_abc123xyz"
}
```

### HTTP Status Codes
| Code | Penggunaan |
|------|-----------|
| 200 | Success (GET, PUT) |
| 201 | Created (POST) |
| 204 | No Content (DELETE) |
| 400 | Bad Request / Validation Error |
| 401 | Unauthorized (token missing/expired) |
| 403 | Forbidden (insufficient scope) |
| 404 | Resource Not Found |
| 409 | Conflict (duplicate, state violation) |
| 422 | Unprocessable Entity (business rule violation) |
| 429 | Too Many Requests |
| 500 | Internal Server Error |
| 503 | Service Unavailable |

---

## 👥 HRIS API

### Employees

#### List Employees
```http
GET /v1/hris/employees
```

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| page | integer | No | Default: 1 |
| per_page | integer | No | Default: 20, Max: 100 |
| department_id | uuid | No | Filter by department |
| status | string | No | ACTIVE, INACTIVE, TERMINATED |
| employment_type | string | No | PERMANENT, CONTRACT, INTERN |
| search | string | No | Search by name or email |
| sort_by | string | No | full_name, hire_date, employee_code |
| sort_order | string | No | asc, desc |

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "employee_code": "EMP00001",
      "full_name": "Budi Santoso",
      "email": "budi.santoso@edcs.internal",
      "department": {
        "id": "dept-uuid",
        "code": "IT",
        "name": "IT & Technology"
      },
      "position": {
        "id": "pos-uuid",
        "name": "Software Engineer",
        "level": "MID"
      },
      "employment_type": "PERMANENT",
      "status": "ACTIVE",
      "hire_date": "2023-03-15",
      "years_of_service": 3.3
    }
  ],
  "meta": { "page": 1, "per_page": 20, "total": 5000 }
}
```

#### Get Employee
```http
GET /v1/hris/employees/{employee_id}
```

#### Create Employee
```http
POST /v1/hris/employees
Content-Type: application/json

{
  "employee_code": "EMP00001",
  "first_name": "Budi",
  "last_name": "Santoso",
  "email": "budi.santoso@edcs.internal",
  "phone": "081234567890",
  "gender": "M",
  "birth_date": "1990-05-20",
  "hire_date": "2026-07-09",
  "department_id": "dept-uuid",
  "position_id": "pos-uuid",
  "employment_type": "PERMANENT",
  "work_location": "Jakarta",
  "manager_id": "manager-uuid"
}
```

#### Update Employee
```http
PATCH /v1/hris/employees/{employee_id}
```

#### Terminate Employee
```http
POST /v1/hris/employees/{employee_id}/terminate
Content-Type: application/json

{
  "termination_date": "2026-07-31",
  "reason": "RESIGNATION",
  "notes": "Pindah ke perusahaan lain"
}
```

### Attendance
```http
GET  /v1/hris/attendance?employee_id={id}&month=2026-07
POST /v1/hris/attendance/check-in
POST /v1/hris/attendance/check-out
GET  /v1/hris/attendance/summary/{employee_id}?month=2026-07
```

### Leave
```http
GET    /v1/hris/leaves?employee_id={id}&status=PENDING
POST   /v1/hris/leaves           # Buat pengajuan
PUT    /v1/hris/leaves/{id}/approve
PUT    /v1/hris/leaves/{id}/reject
DELETE /v1/hris/leaves/{id}      # Cancel (hanya PENDING)
GET    /v1/hris/leaves/balance/{employee_id}
```

### Payroll
```http
POST /v1/hris/payroll/run        # Jalankan payroll
GET  /v1/hris/payroll/runs       # List payroll runs
GET  /v1/hris/payroll/runs/{id}  # Detail run
GET  /v1/hris/payroll/slips/{employee_id}?period=2026-06
GET  /v1/hris/payroll/slips/{id}/download  # Download PDF
```

---

## 💼 CRM API

### Contacts
```http
GET    /v1/crm/contacts?search=budi&segment=VIP
POST   /v1/crm/contacts
GET    /v1/crm/contacts/{id}
PATCH  /v1/crm/contacts/{id}
DELETE /v1/crm/contacts/{id}

# 360° view
GET /v1/crm/contacts/{id}/timeline
GET /v1/crm/contacts/{id}/opportunities
GET /v1/crm/contacts/{id}/tickets
```

### Opportunities
```http
GET   /v1/crm/opportunities?stage=PROPOSAL&owner_id={id}
POST  /v1/crm/opportunities
PATCH /v1/crm/opportunities/{id}
POST  /v1/crm/opportunities/{id}/move-stage
POST  /v1/crm/opportunities/{id}/close-won
POST  /v1/crm/opportunities/{id}/close-lost

# Response untuk close-won:
# → Otomatis trigger sales.order.created event
```

### Tickets
```http
GET   /v1/crm/tickets?status=OPEN&priority=HIGH
POST  /v1/crm/tickets
GET   /v1/crm/tickets/{id}
PATCH /v1/crm/tickets/{id}
POST  /v1/crm/tickets/{id}/assign
POST  /v1/crm/tickets/{id}/resolve
GET   /v1/crm/tickets/{id}/messages
POST  /v1/crm/tickets/{id}/messages  # Tambah reply
```

---

## 🏭 WMS API

### Stock
```http
GET /v1/wms/stock?product_id={id}&warehouse_id={id}
GET /v1/wms/stock/levels                    # Overview semua produk
GET /v1/wms/stock/movements?from=2026-07-01
POST /v1/wms/stock/adjust                   # Manual adjustment

# Request body adjust:
{
  "product_id": "uuid",
  "location_id": "uuid",
  "quantity_delta": -5,
  "reason": "STOCK_TAKE",
  "notes": "Physical count correction"
}
```

### Inbound
```http
POST /v1/wms/receipts           # Terima barang (dari PO)
GET  /v1/wms/receipts/{id}
POST /v1/wms/receipts/{id}/complete
POST /v1/wms/putaway/{receipt_id} # Proses putaway
```

### Outbound
```http
POST /v1/wms/picks              # Buat picking order
GET  /v1/wms/picks/{id}
POST /v1/wms/picks/{id}/start
POST /v1/wms/picks/{id}/complete
POST /v1/wms/shipments          # Buat shipment
POST /v1/wms/shipments/{id}/dispatch
GET  /v1/wms/shipments/{id}/tracking
```

---

## 💰 Finance API

### Journals
```http
GET  /v1/finance/journals?period=2026-06&status=POSTED
POST /v1/finance/journals        # Buat journal entry
GET  /v1/finance/journals/{id}
POST /v1/finance/journals/{id}/post
POST /v1/finance/journals/{id}/reverse

# Request body:
{
  "entry_date": "2026-07-09",
  "description": "Pembayaran vendor PT ABC",
  "reference_type": "AP_PAYMENT",
  "reference_id": "ap-payment-uuid",
  "lines": [
    {
      "account_code": "2100",
      "debit_amount": 0,
      "credit_amount": 5000000,
      "description": "Hutang dagang PT ABC"
    },
    {
      "account_code": "1100",
      "debit_amount": 5000000,
      "credit_amount": 0,
      "description": "Kas keluar"
    }
  ]
}
```

### Reports
```http
GET /v1/finance/reports/trial-balance?period=2026-06
GET /v1/finance/reports/profit-loss?from=2026-01-01&to=2026-06-30
GET /v1/finance/reports/balance-sheet?as_of=2026-06-30
GET /v1/finance/reports/cash-flow?period=2026-06
GET /v1/finance/reports/ar-aging?as_of=2026-07-09
GET /v1/finance/reports/ap-aging?as_of=2026-07-09
```

---

## 📡 IoT API

### Devices
```http
GET  /v1/iot/devices?type=TEMPERATURE&status=ONLINE
POST /v1/iot/devices           # Register device baru
GET  /v1/iot/devices/{id}
GET  /v1/iot/devices/{id}/readings?metric=TEMPERATURE&from=2026-07-09T00:00:00Z
GET  /v1/iot/devices/{id}/alerts?severity=HIGH&resolved=false
POST /v1/iot/devices/{id}/command  # Kirim command ke device
```

### Telemetry
```http
POST /v1/iot/telemetry              # Ingest single reading
POST /v1/iot/telemetry/batch        # Ingest batch (max 1000)
GET  /v1/iot/telemetry/aggregate?device_id={id}&metric=TEMPERATURE&interval=1h&from=...

# Aggregate response:
{
  "device_id": "dev-uuid",
  "metric": "TEMPERATURE",
  "interval": "1h",
  "data": [
    { "timestamp": "2026-07-09T00:00:00Z", "avg": 24.5, "min": 22.1, "max": 27.3, "count": 12 },
    { "timestamp": "2026-07-09T01:00:00Z", "avg": 25.2, "min": 23.0, "max": 28.1, "count": 12 }
  ]
}
```

---

## 🤖 AI API

### Predictions
```http
POST /v1/ai/predict/demand-forecast
POST /v1/ai/predict/churn-employee
POST /v1/ai/predict/maintenance
POST /v1/ai/predict/sales-forecast

# Demand Forecast Request:
{
  "product_ids": ["prod-uuid-1", "prod-uuid-2"],
  "horizon_days": 30,
  "include_confidence_interval": true
}
```

### AI Assistant
```http
POST /v1/ai/chat

# Request:
{
  "session_id": "session-uuid",
  "message": "Berapa saldo cuti tahunan saya?",
  "context": {
    "domain": "HRIS",
    "employee_id": "emp-uuid"
  }
}

# Response:
{
  "response": "Saldo cuti tahunan Anda saat ini adalah 12 hari. ...",
  "sources": [{ "type": "database", "table": "leave_balances" }],
  "session_id": "session-uuid"
}
```

### Document Processing
```http
POST /v1/ai/ocr/invoice          # Upload invoice image → extracted data
POST /v1/ai/summarize            # Summarize document
POST /v1/ai/text-to-sql          # Natural language → SQL query
```

---

## 🔧 Webhook System

```http
POST /v1/webhooks                # Register webhook
GET  /v1/webhooks
DELETE /v1/webhooks/{id}

# Register:
{
  "url": "https://your-app.com/webhooks/edcs",
  "events": ["hris.employee.created", "sales.order.confirmed"],
  "secret": "your-signing-secret"    # HMAC-SHA256 signature
}

# Webhook payload:
POST https://your-app.com/webhooks/edcs
X-EDCS-Signature: sha256=...
X-EDCS-Event: sales.order.confirmed
Content-Type: application/json

{
  "event_id": "evt_uuid",
  "event_type": "sales.order.confirmed",
  "occurred_at": "2026-07-09T10:00:00Z",
  "data": { ... }
}
```
