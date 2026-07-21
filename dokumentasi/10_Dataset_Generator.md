# 10 — Demo Data & IoT Simulator
## Enterprise Digital Platform (EDP)

---

## Overview

EDP tidak memiliki modul "Dataset Generator" terpisah. Data demo dihasilkan melalui dua mekanisme:

1. **Seed data via migration SQL** — data awal yang langsung ada saat service pertama kali dijalankan
2. **IoT Simulator built-in** — bagian dari `iot-service`, menghasilkan sensor readings secara otomatis

---

## Seed Data via Migration

### RBAC (rbac-service)
File migration `backend/services/rbac-service/migrations/`:

| File | Isi |
|------|-----|
| `002_seed.sql` | Role super_admin, company_admin, auditor, branch_manager |
| `003_seed_business_menus.sql` | Menu untuk Finance, HR, Sales, Purchasing, Warehouse, Production, QC, Asset |
| `004_seed_role_permissions.sql` | Permission matrix untuk semua role |
| `005_seed_admin_auditor_permissions.sql` | Permission tambahan |
| `006_seed_menu_icons.sql` | Icon untuk setiap menu |
| `007_seed_finance_coa_menu.sql` | Menu Chart of Accounts |
| `008_seed_warehouse_master_menus.sql` | Menu Products, Warehouses |
| `009_seed_iot_menus.sql` | Menu IoT (Devices, Readings, Alerts) |
| `010_seed_dw_menus.sql` | Menu DW (Sync Status) |

### Auth (auth-service)
`002_seed.sql` — user default: `admin@edp.local` / `Admin@12345` (Super Admin)

---

## Data Demo Manual (via API)

Data berikut dibuat lewat API saat development dan testing, bukan via migration:

- **Finance**: 16 Chart of Accounts, 11 Journal Entries, 11 Invoices
- **HR**: 14 Users (termasuk demo user dengan nama Indonesia)
- **Warehouse**: 2 gudang (WH-A, WH-B), 3 produk (SKU-TEST, SKU-RAW, SKU-FG), stock dari sesi test
- **Production**: 1 BOM, 1 Work Order COMPLETED
- **QC**: 3 inspeksi (PASS, FAIL, PARTIAL)
- **Asset**: 1 aset Forklift, 2 maintenance schedule
- **Company**: Company DEFAULT + Company SECOND (untuk test switcher), 2 branch (HQ, BR-FILTER-TEST)

Data ini hanya ada di database dev lokal, tidak di-commit ke repository.

---

## IoT Simulator

### Cara Kerja

`iot-service` memiliki simulator built-in yang diaktifkan via env var:

```bash
IOT_SIMULATOR_ENABLED=true      # default false di staging/prod
IOT_SIMULATOR_INTERVAL_SECONDS=15  # generate reading tiap 15 detik
```

**Alur simulator**:
```
Ticker (15 detik)
    │
    ▼
Untuk setiap device ACTIVE di database:
    │
    ▼
Generate random reading sesuai device_type:
    TEMPERATURE → random 15-35°C
    HUMIDITY    → random 30-80%
    PRESSURE    → random 990-1020 hPa
    ...
    │
    ▼
Publish ke Mosquitto MQTT (topic: edp/devices/{device_id}/readings)
    │
    ▼
iot-service MQTT subscriber menerima → ingest ke Postgres (readings table)
    │
    ▼
Cek threshold alert (configurable per device)
    │
    ├── Kalau breach > 50% dari range → trigger alert (OPEN)
    └── Publish ke Kafka: iot.alert.triggered (kalau alert baru)
```

### Konfigurasi Device

```json
POST /api/iot/devices
{
    "device_code": "TEMP-001",
    "device_type": "TEMPERATURE",
    "location": "Gudang A",
    "threshold_min": 18.0,
    "threshold_max": 28.0,
    "company_id": "uuid",
    "branch_id": "uuid"
}
```

Device dengan `status=ACTIVE` secara otomatis diikutsertakan dalam simulasi.

### Alert Logic

Alert dipicu kalau reading breach > 50% dari total range threshold:
```
range = threshold_max - threshold_min
breach = max(threshold_min - reading, reading - threshold_max, 0)
fraction = breach / range
if fraction > 0.5 → severity = HIGH
else if fraction > 0 → severity = MEDIUM
```

### Menghentikan Simulator

Set device ke `INACTIVE` atau matikan `IOT_SIMULATOR_ENABLED`:
```bash
PATCH /api/iot/devices/{id}
{"status": "INACTIVE"}
```

---

## Menambah Data Demo Baru

Untuk menambah data demo yang persisten (tidak hilang saat DB di-reset):

1. Tambah file migration baru di service yang relevan (`NNN_seed_demo.sql`)
2. Data akan otomatis ter-apply saat service pertama kali start dengan DB kosong
3. Jangan edit `001_init.sql` yang sudah ada — selalu migration baru

Untuk data one-off (tidak perlu persisten), gunakan API langsung via curl atau frontend.
