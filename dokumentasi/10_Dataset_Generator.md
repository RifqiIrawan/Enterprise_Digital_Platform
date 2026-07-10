# 10 — Dataset Generator
## Enterprise Data Center Simulator (EDCS)

---

## 🎲 Overview

EDCS Dataset Generator adalah modul Python yang menghasilkan **synthetic data realistis** untuk semua modul bisnis. Data dibuat dengan korelasi antar domain (misal: karyawan yang sama muncul di HRIS, sales, dan audit log) untuk mensimulasikan enterprise data center yang sesungguhnya.

---

## 🏗️ Arsitektur Generator

```
┌──────────────────────────────────────────────────────────────┐
│                  DATASET GENERATOR ENGINE                    │
│                                                              │
│  ┌─────────────┐   ┌──────────────┐   ┌──────────────────┐ │
│  │  Faker      │   │  Correlation │   │  Time Series     │ │
│  │  Engine     │   │  Engine      │   │  Engine          │ │
│  │  (PII data) │   │  (cross-mod) │   │  (IoT, trends)   │ │
│  └─────────────┘   └──────────────┘   └──────────────────┘ │
│                                                              │
│  ┌─────────────────────────────────────────────────────────┐│
│  │                OUTPUT ADAPTERS                          ││
│  │  PostgreSQL │ Kafka │ CSV │ JSON │ Parquet │ TimescaleDB││
│  └─────────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────────┘
```

---

## 📦 Project Structure

```
dataset-generator/
├── pyproject.toml
├── config/
│   ├── default.yaml          # Default volume & parameters
│   ├── small.yaml            # 1K employees, 10K orders
│   ├── medium.yaml           # 5K employees, 100K orders
│   └── large.yaml            # 50K employees, 1M orders
├── src/
│   ├── core/
│   │   ├── faker_engine.py   # Wrapper Faker + Locale ID
│   │   ├── correlator.py     # Entity correlation tracker
│   │   ├── time_series.py    # IoT & trend generation
│   │   └── writer.py         # Multi-target output
│   ├── generators/
│   │   ├── erp_generator.py
│   │   ├── hris_generator.py
│   │   ├── crm_generator.py
│   │   ├── sales_generator.py
│   │   ├── wms_generator.py
│   │   ├── mes_generator.py
│   │   ├── finance_generator.py
│   │   ├── procurement_generator.py
│   │   ├── asset_generator.py
│   │   └── iot_generator.py
│   └── cli.py                # CLI entry point
└── tests/
```

---

## ⚙️ Configuration Schema

```yaml
# config/medium.yaml

generation:
  seed: 42                    # Reproducible results
  locale: id_ID               # Indonesian locale
  start_date: "2022-01-01"
  end_date: "2026-06-30"

volumes:
  employees: 5000
  departments: 50
  products: 2000
  customers: 3000
  vendors: 500
  sales_orders: 100000
  purchase_orders: 20000
  work_orders: 30000
  assets: 1500
  iot_devices: 500
  iot_readings_per_device_per_day: 288  # setiap 5 menit

output:
  targets:
    - type: postgresql
      services: [erp, hris, crm, sales, wms, mes, finance, procurement, asset]
    - type: kafka
      brokers: "kafka:9092"
      topics:
        realtime: [hris, iot, sales]
    - type: parquet
      path: /data/synthetic/
      partition_by: domain

business_rules:
  employee_turnover_rate: 0.15      # 15% churn per tahun
  sales_growth_rate: 0.20           # 20% growth YoY
  inventory_stockout_rate: 0.05     # 5% item stockout
  quality_defect_rate: 0.02         # 2% defect rate
  payment_on_time_rate: 0.85        # 85% payment on time
```

---

## 🐍 Core Generators

### HRIS Generator
```python
# src/generators/hris_generator.py
import random
from faker import Faker
from datetime import date, timedelta
from typing import List, Dict

fake = Faker("id_ID")

# Departemen Indonesia realistis
DEPARTMENTS = [
    ("TECH", "IT & Technology", ["Software Engineer", "DevOps", "Data Engineer"]),
    ("FIN",  "Finance & Accounting", ["Akuntan", "Financial Analyst", "Controller"]),
    ("HRD",  "Human Resources", ["HR Generalist", "Recruiter", "Payroll Specialist"]),
    ("OPS",  "Operations", ["Operations Manager", "Supervisor", "Operator"]),
    ("SCM",  "Supply Chain", ["Buyer", "Warehouse Staff", "Logistics Coordinator"]),
    ("SALES","Sales & Marketing", ["Sales Executive", "Account Manager", "Marketing Specialist"]),
    ("MFG",  "Manufacturing", ["Production Supervisor", "Quality Control", "Maintenance"]),
    ("IT",   "IT Infrastructure", ["System Admin", "Network Engineer", "Security Analyst"]),
]

EMPLOYMENT_TYPES = ["PERMANENT"] * 70 + ["CONTRACT"] * 20 + ["INTERN"] * 8 + ["OUTSOURCE"] * 2

def generate_employees(n: int, start_date: date) -> List[Dict]:
    employees = []
    for i in range(n):
        dept_code, dept_name, positions = random.choice(DEPARTMENTS)
        hire_date = fake.date_between(start_date=start_date, end_date="today")

        # Simulasi turnover
        is_terminated = random.random() < 0.15
        termination_date = None
        if is_terminated and hire_date < date.today() - timedelta(days=90):
            termination_date = fake.date_between(
                start_date=hire_date + timedelta(days=90),
                end_date="today"
            )

        gender = random.choices(["M", "F"], weights=[55, 45])[0]
        emp = {
            "id": fake.uuid4(),
            "employee_code": f"EMP{str(i+1).zfill(5)}",
            "first_name": fake.first_name_male() if gender == "M" else fake.first_name_female(),
            "last_name": fake.last_name(),
            "email": f"emp{i+1}@edcs.internal",
            "phone": fake.phone_number(),
            "nik": fake.numerify("################"),
            "gender": gender,
            "birth_date": fake.date_of_birth(minimum_age=22, maximum_age=55),
            "hire_date": hire_date,
            "termination_date": termination_date,
            "status": "TERMINATED" if is_terminated else "ACTIVE",
            "department_code": dept_code,
            "department_name": dept_name,
            "position_name": random.choice(positions),
            "employment_type": random.choice(EMPLOYMENT_TYPES),
            "work_location": random.choice(["Jakarta", "Bandung", "Surabaya", "Bali", "WFH"]),
            "basic_salary": round(random.gauss(8_000_000, 3_000_000) / 100_000) * 100_000,
        }
        employees.append(emp)

    return employees


def generate_attendance(employees: List[Dict], year: int, month: int) -> List[Dict]:
    """Generate realistic attendance patterns including late, absent, etc."""
    records = []
    import calendar
    _, days_in_month = calendar.monthrange(year, month)

    for emp in employees:
        if emp["status"] != "ACTIVE":
            continue

        for day in range(1, days_in_month + 1):
            log_date = date(year, month, day)
            if log_date.weekday() >= 5:  # Skip weekend
                continue

            # Random attendance patterns
            rand = random.random()
            if rand < 0.02:      # 2% absent
                status = "ABSENT"
                check_in = check_out = None
            elif rand < 0.08:    # 6% late
                status = "LATE"
                check_in = fake.date_time_this_month().replace(
                    hour=random.randint(9, 11), minute=random.randint(0, 59))
                check_out = check_in.replace(hour=18, minute=0)
            else:                # Normal
                status = "PRESENT"
                check_in = fake.date_time_this_month().replace(
                    hour=8, minute=random.randint(0, 30))
                check_out = check_in.replace(hour=random.randint(17, 20))

            records.append({
                "id": fake.uuid4(),
                "employee_id": emp["id"],
                "log_date": log_date.isoformat(),
                "check_in": check_in.isoformat() if check_in else None,
                "check_out": check_out.isoformat() if check_out else None,
                "status": status,
                "source": random.choice(["BIOMETRIC", "QR", "MANUAL"]),
            })

    return records
```

### IoT Time Series Generator
```python
# src/generators/iot_generator.py
import numpy as np
from datetime import datetime, timedelta

def generate_sensor_readings(
    device_id: str,
    sensor_type: str,
    start: datetime,
    end: datetime,
    interval_minutes: int = 5
) -> list:
    """Generate realistic sensor readings with trends, noise, and anomalies."""

    # Base parameters per sensor type
    params = {
        "TEMPERATURE": {"mean": 25.0, "std": 2.0, "unit": "°C", "anomaly_value": 85.0},
        "HUMIDITY":    {"mean": 60.0, "std": 5.0, "unit": "%",  "anomaly_value": 95.0},
        "PRESSURE":    {"mean": 101.3,"std": 0.5, "unit": "kPa","anomaly_value": 95.0},
        "VIBRATION":   {"mean": 0.5,  "std": 0.1, "unit": "mm/s","anomaly_value": 5.0},
        "POWER":       {"mean": 220.0,"std": 5.0, "unit": "V",  "anomaly_value": 260.0},
        "RPM":         {"mean": 1500, "std": 50,  "unit": "rpm","anomaly_value": 3000},
    }

    p = params.get(sensor_type, {"mean": 50.0, "std": 5.0, "unit": "", "anomaly_value": 100.0})
    readings = []
    current = start
    anomaly_rate = 0.005  # 0.5% anomaly

    while current <= end:
        hour = current.hour
        # Sinusoidal daily pattern (mesin lebih panas siang hari)
        daily_factor = 1 + 0.1 * np.sin(np.pi * (hour - 6) / 12)

        # Gaussian noise
        value = np.random.normal(p["mean"] * daily_factor, p["std"])

        # Inject anomaly
        if np.random.random() < anomaly_rate:
            value = p["anomaly_value"] + np.random.normal(0, p["std"])

        readings.append({
            "time": current.isoformat(),
            "device_id": device_id,
            "metric_name": sensor_type,
            "metric_value": round(float(value), 4),
            "unit": p["unit"],
            "quality": 100 if abs(value - p["mean"]) < 3 * p["std"] else 50,
        })

        current += timedelta(minutes=interval_minutes)

    return readings
```

---

## 🚀 CLI Usage

```bash
# Generate data set kecil untuk development
python -m edcs_generator generate --config config/small.yaml --output postgresql

# Generate IoT streaming ke Kafka (real-time)
python -m edcs_generator stream --devices 100 --interval 5 --target kafka

# Generate CSV untuk import manual
python -m edcs_generator generate --config config/medium.yaml --output csv --path /data/export/

# Reset semua database dan regenerate
python -m edcs_generator reset-and-seed --config config/medium.yaml

# Generate hanya domain tertentu
python -m edcs_generator generate --domain hris --employees 1000 --months 24

# Status dan validasi
python -m edcs_generator validate --domain all
```

---

## 📊 Generated Data Volume (Medium Config)

| Domain | Entity | Record Count | Size (est.) |
|--------|--------|-------------|-------------|
| HRIS | Employees | 5.000 | 5 MB |
| HRIS | Attendance | 1.200.000 | 300 MB |
| HRIS | Payroll Runs | 120.000 | 50 MB |
| CRM | Contacts | 3.000 | 2 MB |
| CRM | Opportunities | 15.000 | 10 MB |
| Sales | Orders | 100.000 | 80 MB |
| WMS | Stock Movements | 500.000 | 200 MB |
| MES | Work Orders | 30.000 | 30 MB |
| Finance | Journal Lines | 800.000 | 400 MB |
| Procurement | POs | 20.000 | 15 MB |
| IoT | Sensor Readings | 72.000.000 | 15 GB |
| **Total** | | **~75 juta records** | **~16 GB** |
