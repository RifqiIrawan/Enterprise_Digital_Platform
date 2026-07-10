# 06 — Data Lake Architecture
## Enterprise Data Center Simulator (EDCS)

---

## 🏛️ Overview

EDCS Data Lake mengimplementasikan **Medallion Architecture** (Bronze → Silver → Gold) di atas **MinIO** (S3-compatible object storage) dengan **Delta Lake** sebagai format tabel terbuka yang mendukung ACID transactions, time travel, dan schema evolution.

---

## 🥉🥈🥇 Medallion Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    BRONZE LAYER (Raw)                       │
│  Format: Parquet / JSON / Avro (as-is dari source)         │
│  Retention: 5 tahun                                         │
│  Schema: Schema-on-read                                     │
│  Processing: Tidak ada transformasi                         │
│  Path: s3://edcs-lake/bronze/{domain}/{table}/{yyyy/mm/dd}  │
└──────────────────────────┬──────────────────────────────────┘
                           │ (Spark ETL / dbt)
┌──────────────────────────▼──────────────────────────────────┐
│                    SILVER LAYER (Curated)                   │
│  Format: Delta Lake                                         │
│  Retention: 3 tahun                                         │
│  Schema: Schema enforced                                    │
│  Processing: Cleansed, deduplicated, standardized           │
│  Path: s3://edcs-lake/silver/{domain}/{table}/              │
└──────────────────────────┬──────────────────────────────────┘
                           │ (Spark / dbt)
┌──────────────────────────▼──────────────────────────────────┐
│                    GOLD LAYER (Enriched)                    │
│  Format: Delta Lake (partitioned, Z-ordered)                │
│  Retention: 1 tahun (aktif), archive 5 tahun               │
│  Schema: Business-ready                                     │
│  Processing: Aggregated, joined, business logic applied     │
│  Path: s3://edcs-lake/gold/{use_case}/                      │
└─────────────────────────────────────────────────────────────┘
```

---

## 📁 Direktori Struktur Data Lake

```
s3://edcs-lake/
├── bronze/
│   ├── erp/
│   │   ├── master_data/
│   │   ├── accounts/
│   │   └── cost_centers/
│   ├── hris/
│   │   ├── employees/
│   │   ├── attendance/
│   │   └── payroll_runs/
│   ├── crm/
│   │   ├── contacts/
│   │   ├── opportunities/
│   │   └── tickets/
│   ├── sales/
│   │   └── orders/
│   ├── wms/
│   │   ├── stock_levels/
│   │   └── stock_movements/
│   ├── mes/
│   │   ├── work_orders/
│   │   └── quality_checks/
│   ├── finance/
│   │   ├── journal_entries/
│   │   ├── ap_invoices/
│   │   └── ar_invoices/
│   ├── procurement/
│   │   └── purchase_orders/
│   ├── iot/
│   │   ├── sensor_readings/    # Partisi per jam
│   │   └── device_events/
│   └── _metadata/
│       └── ingestion_logs/
│
├── silver/
│   ├── hris/
│   │   ├── employees/          # Delta table
│   │   └── attendance_daily/   # Delta table
│   ├── sales/
│   │   └── orders_enriched/
│   ├── iot/
│   │   └── sensor_readings_clean/
│   └── finance/
│       └── transactions_normalized/
│
├── gold/
│   ├── sales_analytics/
│   ├── hr_analytics/
│   ├── operations_analytics/
│   ├── finance_analytics/
│   └── iot_analytics/
│
└── _system/
    ├── checkpoints/            # Spark streaming checkpoints
    ├── schemas/                # Schema registry backup
    └── quarantine/             # Bad data records
```

---

## 🔄 Data Ingestion Pipelines

### Batch Ingestion (CDC via Debezium → Kafka → Lake)
```python
# Spark job: bronze_ingestion.py

from pyspark.sql import SparkSession
from delta import configure_spark_with_delta_pip

spark = (SparkSession.builder
    .appName("EDCS-Bronze-Ingestion")
    .config("spark.sql.extensions", "io.delta.sql.DeltaSparkSessionExtension")
    .config("spark.sql.catalog.spark_catalog",
            "org.apache.spark.sql.delta.catalog.DeltaCatalog")
    .getOrCreate())

# Baca dari Kafka topic
df = (spark.readStream
    .format("kafka")
    .option("kafka.bootstrap.servers", "kafka:9092")
    .option("subscribe", "hris.employees")
    .option("startingOffsets", "latest")
    .load())

# Parse Avro payload
from pyspark.sql.avro.functions import from_avro
schema_str = get_schema_from_registry("hris.employees")
parsed = df.select(
    from_avro("value", schema_str).alias("data"),
    "timestamp", "partition", "offset"
).select("data.*", "timestamp")

# Write ke Bronze (append-only, dengan watermark)
query = (parsed.writeStream
    .format("delta")
    .outputMode("append")
    .option("checkpointLocation", "s3://edcs-lake/_system/checkpoints/hris_employees")
    .option("path", "s3://edcs-lake/bronze/hris/employees/")
    .partitionBy("_ingestion_date")
    .trigger(processingTime="5 minutes")
    .start())
```

### Silver Layer Transformation
```python
# Spark job: silver_hris_employees.py
from pyspark.sql import functions as F

bronze_df = spark.read.format("delta") \
    .load("s3://edcs-lake/bronze/hris/employees/")

silver_df = (bronze_df
    # Deduplication: ambil record terbaru per employee_id
    .withColumn("row_num", F.row_number().over(
        Window.partitionBy("employee_id")
              .orderBy(F.desc("updated_at"))))
    .filter(F.col("row_num") == 1)
    # Standardisasi
    .withColumn("full_name",
        F.trim(F.concat_ws(" ", "first_name", "last_name")))
    .withColumn("email", F.lower("email"))
    .withColumn("hire_date", F.to_date("hire_date"))
    # Hapus kolom teknis
    .drop("row_num", "_kafka_offset", "_ingestion_ts")
    # Tambah metadata silver
    .withColumn("_silver_processed_at", F.current_timestamp())
    .withColumn("_source", F.lit("hris"))
)

# Upsert ke Silver (MERGE untuk idempotency)
from delta.tables import DeltaTable

if DeltaTable.isDeltaTable(spark, "s3://edcs-lake/silver/hris/employees/"):
    delta_table = DeltaTable.forPath(spark, "s3://edcs-lake/silver/hris/employees/")
    delta_table.alias("silver").merge(
        silver_df.alias("updates"),
        "silver.employee_id = updates.employee_id"
    ).whenMatchedUpdateAll() \
     .whenNotMatchedInsertAll() \
     .execute()
else:
    silver_df.write.format("delta") \
        .mode("overwrite") \
        .save("s3://edcs-lake/silver/hris/employees/")
```

---

## 📊 Data Lake Catalog (Apache Atlas / OpenMetadata)

### Metadata yang Dicapture per Dataset
```yaml
dataset:
  name: "silver.hris.employees"
  description: "Cleansed employee master data dari HRIS"
  owner: "data-engineering@edcs.internal"
  domain: "HRIS"
  layer: "silver"
  format: "delta"
  location: "s3://edcs-lake/silver/hris/employees/"
  update_frequency: "every 15 minutes (streaming)"
  row_count: 5200
  size_gb: 0.8
  schema:
    - name: employee_id
      type: uuid
      description: "Primary key dari HRIS"
      pii: false
    - name: email
      type: string
      description: "Email karyawan"
      pii: true
      classification: "CONFIDENTIAL"
  lineage:
    upstream:
      - "bronze.hris.employees (Debezium CDC)"
    downstream:
      - "gold.hr_analytics.headcount_daily"
      - "data_warehouse.dim_employee"
  quality_score: 98.5
  last_updated: "2026-07-09T03:00:00Z"
```

---

## 🛡️ Data Governance

### Data Classification
| Level | Contoh Data | Kontrol |
|-------|-------------|---------|
| **PUBLIC** | Product catalog, pricing | Akses terbuka |
| **INTERNAL** | Sales metrics, inventory | Autentikasi |
| **CONFIDENTIAL** | Gaji, data keuangan | Role-based access |
| **RESTRICTED** | NIK, nomor rekening | Enkripsi + masking |

### PII Masking Rules
```python
# Transformasi PII di Silver Layer
pii_masking_rules = {
    "nik": lambda col: F.concat(F.lit("****"), F.substring(col, 13, 4)),
    "phone": lambda col: F.regexp_replace(col, r"(\d{4})(\d+)(\d{4})", r"\1****\3"),
    "bank_account": lambda col: F.lit("***MASKED***"),
    "email": lambda col: F.regexp_replace(col, r"(.{2})(.*)(@.*)", r"\1***\3"),
}
```

### Retention Policy
| Layer | Domain | Retention |
|-------|--------|-----------|
| Bronze | IoT Sensor | 1 tahun |
| Bronze | Business Events | 5 tahun |
| Bronze | Finance | 7 tahun (regulasi) |
| Silver | Semua | 3 tahun |
| Gold | Semua | 1 tahun aktif |
