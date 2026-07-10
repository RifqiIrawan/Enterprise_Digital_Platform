# 08 — ETL/ELT Pipeline
## Enterprise Data Center Simulator (EDCS)

---

## 🔄 Pipeline Philosophy

EDCS mengadopsi **ELT (Extract-Load-Transform)** sebagai pendekatan utama — data di-load dulu ke Data Lake dalam bentuk mentah (Bronze), baru ditransformasi menggunakan Spark & dbt. ETL tradisional hanya digunakan untuk kasus khusus (masking PII sebelum landing).

```
ETL (lama)          ELT (EDCS)
────────────        ─────────────────────────────
Extract             Extract
Transform           Load (Raw ke Bronze)
Load                Transform (Bronze→Silver→Gold)
```

---

## 🏗️ Pipeline Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   SOURCE SYSTEMS                            │
└──────────┬──────────────┬──────────────┬───────────────────┘
           │              │              │
     CDC (Debezium)   API Pull      File Upload
           │              │              │
           ▼              ▼              ▼
┌─────────────────────────────────────────────────────────────┐
│                    INGESTION LAYER                          │
│         Kafka Connect │ Airbyte │ Custom Connectors         │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│              MESSAGE BROKER (Kafka)                         │
│    Topics dengan schema validation (Avro + Registry)        │
└──────────────────────────┬──────────────────────────────────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
         Spark         Spark          Flink
         Batch         Streaming      CEP
              │            │            │
              └────────────▼────────────┘
                    Data Lake (MinIO)
                    Bronze → Silver → Gold
                           │
                           ▼
                    ┌─────────────┐
                    │   dbt       │
                    │ (SQL xform) │
                    └──────┬──────┘
                           │
                           ▼
                    Data Warehouse
                    (ClickHouse / DuckDB)
```

---

## 🔌 Kafka Connect: Source Connectors

### 1. PostgreSQL CDC (Debezium)
```json
{
  "name": "hris-postgres-cdc",
  "config": {
    "connector.class": "io.debezium.connector.postgresql.PostgresConnector",
    "database.hostname": "postgres-hris",
    "database.port": "5432",
    "database.user": "debezium",
    "database.password": "${DEBEZIUM_PASS}",
    "database.dbname": "hris_db",
    "database.server.name": "hris",
    "table.include.list": "public.employees,public.attendance_logs,public.leave_requests,public.payroll_runs",
    "plugin.name": "pgoutput",
    "publication.name": "debezium_pub",
    "slot.name": "debezium_hris_slot",
    "key.converter": "io.confluent.kafka.serializers.KafkaAvroSerializer",
    "value.converter": "io.confluent.kafka.serializers.KafkaAvroSerializer",
    "schema.registry.url": "http://schema-registry:8081",
    "transforms": "unwrap",
    "transforms.unwrap.type": "io.debezium.transforms.ExtractNewRecordState",
    "transforms.unwrap.add.fields": "op,ts_ms,source.table",
    "heartbeat.interval.ms": "10000"
  }
}
```

### 2. MQTT → Kafka (IoT)
```json
{
  "name": "iot-mqtt-source",
  "config": {
    "connector.class": "io.confluent.connect.mqtt.MqttSourceConnector",
    "mqtt.server.uri": "tcp://emqx:1883",
    "mqtt.topics": "edcs/+/sensors/#",
    "kafka.topic": "iot.sensor.raw",
    "mqtt.qos": "1",
    "mqtt.username": "kafka-connect",
    "mqtt.password": "${MQTT_PASS}",
    "message.processor.class": "io.confluent.connect.mqtt.processors.JsonMessageProcessor"
  }
}
```

---

## 🔌 Kafka Connect: Sink Connectors

### S3/MinIO Sink (Bronze Layer)
```json
{
  "name": "bronze-s3-sink-hris",
  "config": {
    "connector.class": "io.confluent.connect.s3.S3SinkConnector",
    "tasks.max": "4",
    "topics": "hris.employees,hris.attendance_logs",
    "s3.region": "us-east-1",
    "s3.bucket.name": "edcs-lake",
    "s3.part.size": "5242880",
    "topics.dir": "bronze/hris",
    "flush.size": "10000",
    "rotate.interval.ms": "300000",
    "rotate.schedule.interval.ms": "600000",
    "storage.class": "io.confluent.connect.s3.storage.S3Storage",
    "format.class": "io.confluent.connect.s3.format.parquet.ParquetFormat",
    "parquet.codec": "snappy",
    "locale": "id_ID",
    "timezone": "Asia/Jakarta",
    "timestamp.extractor": "RecordField",
    "timestamp.field": "updated_at",
    "partitioner.class": "io.confluent.connect.storage.partitioner.TimeBasedPartitioner",
    "path.format": "'year'=YYYY/'month'=MM/'day'=dd",
    "partition.duration.ms": "86400000"
  }
}
```

---

## ⚙️ dbt Transformation Patterns

### Pattern 1: Staging (stg_) — Minimal Transform
```sql
-- models/staging/hris/stg_hris__employees.sql
{{ config(materialized='incremental', unique_key='employee_id') }}

WITH source AS (
    SELECT * FROM {{ source('hris_bronze', 'employees') }}
    {% if is_incremental() %}
    WHERE _ingested_at > (SELECT MAX(_ingested_at) FROM {{ this }})
    {% endif %}
),

renamed AS (
    SELECT
        id::uuid               AS employee_id,
        employee_code,
        TRIM(first_name)       AS first_name,
        TRIM(last_name)        AS last_name,
        LOWER(email)           AS email,
        phone,
        gender,
        hire_date::date        AS hire_date,
        termination_date::date AS termination_date,
        status,
        department_id::uuid    AS department_id,
        position_id::uuid      AS position_id,
        is_active,
        created_at,
        updated_at,
        _op                    AS cdc_operation,  -- I/U/D dari Debezium
        _ts_ms                 AS cdc_timestamp,
        CURRENT_TIMESTAMP      AS _ingested_at
    FROM source
    WHERE _op != 'D'  -- exclude soft deletes dari stream
)

SELECT * FROM renamed
```

### Pattern 2: Intermediate (int_) — Business Logic
```sql
-- models/intermediate/int_employees_enriched.sql
{{ config(materialized='table') }}

WITH employees AS (
    SELECT * FROM {{ ref('stg_hris__employees') }}
    WHERE is_active = TRUE
),

departments AS (
    SELECT * FROM {{ ref('stg_hris__departments') }}
),

positions AS (
    SELECT * FROM {{ ref('stg_hris__positions') }}
),

managers AS (
    SELECT
        employee_id AS manager_id,
        full_name   AS manager_name,
        email       AS manager_email
    FROM {{ ref('stg_hris__employees') }}
),

enriched AS (
    SELECT
        e.*,
        CONCAT_WS(' ', e.first_name, e.last_name) AS full_name,
        d.name     AS department_name,
        d.code     AS department_code,
        p.name     AS position_name,
        p.level    AS position_level,
        m.manager_name,
        -- Computed fields
        DATE_PART('year', AGE(NOW(), e.hire_date)) AS years_of_service,
        CASE
            WHEN DATE_PART('year', AGE(NOW(), e.hire_date)) >= 10 THEN 'VETERAN'
            WHEN DATE_PART('year', AGE(NOW(), e.hire_date)) >= 5  THEN 'SENIOR'
            WHEN DATE_PART('year', AGE(NOW(), e.hire_date)) >= 2  THEN 'MID'
            ELSE 'JUNIOR'
        END AS tenure_band
    FROM employees e
    LEFT JOIN departments d  ON e.department_id = d.department_id
    LEFT JOIN positions p    ON e.position_id   = p.position_id
    LEFT JOIN managers m     ON e.manager_id    = m.manager_id
)

SELECT * FROM enriched
```

### Pattern 3: Mart (mart_) — Final Output
```sql
-- models/marts/hr/mart_headcount_monthly.sql
{{ config(materialized='table') }}

WITH base AS (
    SELECT * FROM {{ ref('int_employees_enriched') }}
),

months AS (
    SELECT DISTINCT
        DATE_TRUNC('month', generate_series) AS month_start
    FROM GENERATE_SERIES(
        (SELECT MIN(hire_date) FROM base),
        CURRENT_DATE,
        '1 month'::interval
    ) AS gs(generate_series)
),

headcount AS (
    SELECT
        m.month_start,
        TO_CHAR(m.month_start, 'YYYY-MM') AS month_period,
        e.department_name,
        e.position_level,
        e.employment_type,
        COUNT(*) AS headcount,
        COUNT(*) FILTER (WHERE e.gender = 'M') AS male_count,
        COUNT(*) FILTER (WHERE e.gender = 'F') AS female_count,
        AVG(e.years_of_service) AS avg_years_service,
        SUM(CASE WHEN e.tenure_band = 'JUNIOR' THEN 1 ELSE 0 END) AS junior_count,
        SUM(CASE WHEN e.tenure_band = 'SENIOR' THEN 1 ELSE 0 END) AS senior_count
    FROM months m
    JOIN base e
        ON e.hire_date <= m.month_start
        AND (e.termination_date IS NULL OR e.termination_date > m.month_start)
    GROUP BY 1, 2, 3, 4, 5
)

SELECT * FROM headcount
ORDER BY month_start, department_name
```

---

## 🧪 Data Quality Tests (dbt)

```yaml
# models/staging/hris/schema.yml
version: 2

models:
  - name: stg_hris__employees
    description: "Staged employee data dari HRIS"
    columns:
      - name: employee_id
        tests:
          - unique
          - not_null
      - name: email
        tests:
          - unique
          - not_null
          - accepted_format:
              regex: "^[a-z0-9._%+-]+@[a-z0-9.-]+\\.[a-z]{2,}$"
      - name: gender
        tests:
          - accepted_values:
              values: ['M', 'F']
      - name: status
        tests:
          - accepted_values:
              values: ['ACTIVE', 'INACTIVE', 'TERMINATED', 'ON_LEAVE']
      - name: hire_date
        tests:
          - not_null
          - dbt_expectations.expect_column_values_to_be_between:
              min_value: "'2000-01-01'"
              max_value: "CURRENT_DATE"
```

---

## 📅 Pipeline SLA & Monitoring

| Pipeline | Sumber | Target | Latency SLA | Alert jika |
|----------|--------|--------|-------------|------------|
| HRIS CDC | PostgreSQL | Bronze | < 30 detik | > 2 menit |
| IoT Stream | MQTT | Bronze | < 5 detik | > 30 detik |
| Silver HRIS | Bronze | Silver | < 10 menit | > 30 menit |
| Gold Analytics | Silver | Gold | < 30 menit | > 1 jam |
| DWH Load | Gold | ClickHouse | < 1 jam | > 2 jam |
| dbt Run | Multiple | DWH | < 2 jam | > 3 jam |
