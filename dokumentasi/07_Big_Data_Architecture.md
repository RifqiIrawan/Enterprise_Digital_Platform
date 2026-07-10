# 07 — Big Data Architecture
## Enterprise Data Center Simulator (EDCS)

---

## 🐘 Overview

EDCS Big Data layer menggunakan **Apache Spark** sebagai distributed processing engine, dipadukan dengan **Apache Kafka** untuk streaming, **Apache Airflow** untuk orchestrasi, dan **Trino** untuk federated query engine yang menyatukan semua data source.

---

## 🏗️ Big Data Stack

```
┌─────────────────────────────────────────────────────────────┐
│                   DATA SOURCES                              │
│  PostgreSQL DBs │ IoT (TimescaleDB) │ Files │ APIs          │
└──────┬──────────────────┬──────────────────┬───────────────┘
       │ CDC (Debezium)   │ MQTT             │ REST Pull
       ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────────┐
│                STREAMING LAYER (Kafka)                      │
│  Schema Registry │ Kafka Connect │ Kafka Streams            │
└──────────────────────────┬──────────────────────────────────┘
                           │
         ┌─────────────────┼─────────────────┐
         ▼                 ▼                 ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  Spark       │  │  Flink       │  │  Kafka       │
│  (Batch)     │  │  (Stream)    │  │  Streams     │
│  ETL, ML     │  │  CEP, Alert  │  │  Simple      │
│  Large jobs  │  │  Low-latency │  │  Aggregation │
└──────┬───────┘  └──────┬───────┘  └──────┬───────┘
       │                 │                 │
       └─────────────────▼─────────────────┘
                  Data Lake (MinIO/Delta)
                         │
                         ▼
              ┌──────────────────────┐
              │  QUERY ENGINE        │
              │  Trino (Federated)   │
              │  - Delta Lake        │
              │  - PostgreSQL        │
              │  - Elasticsearch     │
              │  - Kafka (live)      │
              └──────────────────────┘
                         │
                         ▼
              BI Tools / ML Platform / APIs
```

---

## ⚡ Apache Spark Cluster

### Cluster Configuration
```yaml
# spark-defaults.conf
spark.master                    k8s://https://kubernetes:6443
spark.kubernetes.namespace      edcs-data
spark.kubernetes.driver.pod.name spark-driver

# Resource configuration
spark.driver.memory             4g
spark.driver.cores              2
spark.executor.memory           8g
spark.executor.cores            4
spark.executor.instances        5    # Dynamic: 2-20

# Dynamic Allocation
spark.dynamicAllocation.enabled              true
spark.dynamicAllocation.minExecutors         2
spark.dynamicAllocation.maxExecutors         20
spark.dynamicAllocation.executorIdleTimeout  60s

# Delta Lake
spark.sql.extensions            io.delta.sql.DeltaSparkSessionExtension
spark.sql.catalog.spark_catalog org.apache.spark.sql.delta.catalog.DeltaCatalog

# S3/MinIO
spark.hadoop.fs.s3a.endpoint    http://minio:9000
spark.hadoop.fs.s3a.access.key  ${MINIO_ACCESS_KEY}
spark.hadoop.fs.s3a.secret.key  ${MINIO_SECRET_KEY}
spark.hadoop.fs.s3a.path.style.access true
```

### Spark Job Categories

#### 1. Batch Ingestion Jobs (Bronze layer)
```python
# Contoh: full load dari PostgreSQL
class FullLoadJob:
    def __init__(self, source_table: str, target_path: str):
        self.source = source_table
        self.target = target_path

    def run(self, spark: SparkSession):
        df = spark.read.format("jdbc") \
            .option("url", os.environ["HRIS_DB_URL"]) \
            .option("dbtable", self.source) \
            .option("numPartitions", 10) \
            .option("partitionColumn", "id") \
            .option("lowerBound", 0) \
            .option("upperBound", 1000000) \
            .load()

        df.write.format("delta") \
            .mode("overwrite") \
            .option("overwriteSchema", "true") \
            .save(self.target)

        return df.count()
```

#### 2. Streaming Jobs (Silver layer, real-time)
```python
# Contoh: IoT sensor processing
def process_iot_stream(spark: SparkSession):
    schema = StructType([
        StructField("device_id", StringType()),
        StructField("metric_name", StringType()),
        StructField("metric_value", DoubleType()),
        StructField("timestamp", TimestampType())
    ])

    raw_stream = spark.readStream \
        .format("kafka") \
        .option("kafka.bootstrap.servers", "kafka:9092") \
        .option("subscribe", "iot.sensor.reading") \
        .load()

    parsed = raw_stream.select(
        F.from_json(F.col("value").cast("string"), schema).alias("data"),
        F.col("timestamp").alias("kafka_ts")
    ).select("data.*", "kafka_ts")

    # Watermark untuk late data tolerance
    with_watermark = parsed \
        .withWatermark("timestamp", "2 minutes") \
        .withColumn("window", F.window("timestamp", "5 minutes"))

    # Agregasi per device per window
    aggregated = with_watermark.groupBy("device_id", "metric_name", "window") \
        .agg(
            F.avg("metric_value").alias("avg_value"),
            F.max("metric_value").alias("max_value"),
            F.min("metric_value").alias("min_value"),
            F.count("*").alias("reading_count")
        )

    return aggregated.writeStream \
        .format("delta") \
        .outputMode("append") \
        .option("checkpointLocation", "s3://edcs-lake/_system/checkpoints/iot_agg") \
        .option("path", "s3://edcs-lake/silver/iot/sensor_aggregated/") \
        .start()
```

---

## 🌊 Apache Kafka Setup

### Kafka Cluster (KRaft mode, no Zookeeper)
```yaml
# kafka/kraft-config.properties
node.id=1
process.roles=broker,controller
controller.quorum.voters=1@kafka-0:9093,2@kafka-1:9093,3@kafka-2:9093
listeners=PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093
advertised.listeners=PLAINTEXT://kafka-0:9092

# Retention
log.retention.hours=168        # 7 hari
log.retention.bytes=107374182400  # 100 GB
log.segment.bytes=1073741824   # 1 GB

# Performance
num.partitions=6
default.replication.factor=3
min.insync.replicas=2
```

### Schema Registry (Confluent)
```json
// Schema: hris.EmployeeCreatedEvent (Avro)
{
  "type": "record",
  "name": "EmployeeCreatedEvent",
  "namespace": "com.edcs.hris",
  "fields": [
    {"name": "event_id",     "type": "string"},
    {"name": "event_type",   "type": "string", "default": "EMPLOYEE_CREATED"},
    {"name": "occurred_at",  "type": "long",   "logicalType": "timestamp-millis"},
    {"name": "employee_id",  "type": "string"},
    {"name": "employee_code","type": "string"},
    {"name": "full_name",    "type": "string"},
    {"name": "email",        "type": "string"},
    {"name": "department",   "type": ["null", "string"]},
    {"name": "hire_date",    "type": "int",    "logicalType": "date"}
  ]
}
```

---

## 🔍 Trino Federated Query Engine

### Catalog Configuration
```properties
# /etc/trino/catalog/delta.properties
connector.name=delta
hive.metastore.uri=thrift://hive-metastore:9083
hive.s3.endpoint=http://minio:9000
hive.s3.path-style-access=true

# /etc/trino/catalog/postgresql.properties
connector.name=postgresql
connection-url=jdbc:postgresql://postgres-erp:5432/erp_db
connection-user=${ERP_DB_USER}
connection-password=${ERP_DB_PASS}

# /etc/trino/catalog/elasticsearch.properties
connector.name=elasticsearch
elasticsearch.host=elasticsearch
elasticsearch.port=9200
```

### Contoh Federated Query
```sql
-- Join data dari PostgreSQL (operational) + Delta Lake (analytical)
SELECT
    e.employee_code,
    e.full_name,
    s.month_period,
    s.net_salary,
    a.avg_attendance_days
FROM postgresql.hris_db.employees e
JOIN delta.silver_hris.payroll_monthly s
    ON e.id = s.employee_id
JOIN delta.gold_hr.attendance_summary a
    ON e.id = a.employee_id
    AND s.month_period = a.month_period
WHERE s.month_period = '2026-06'
ORDER BY s.net_salary DESC
LIMIT 100;
```

---

## 📊 Big Data Job Registry (Airflow)

```python
# DAG: edcs_big_data_daily
from airflow import DAG
from airflow.providers.apache.spark.operators.spark_submit import SparkSubmitOperator

with DAG("edcs_big_data_daily", schedule_interval="0 1 * * *") as dag:

    bronze_erp = SparkSubmitOperator(
        task_id="bronze_erp_incremental",
        application="jobs/bronze/erp_incremental.py",
        conf={"spark.executor.instances": "3"},
    )

    bronze_hris = SparkSubmitOperator(
        task_id="bronze_hris_incremental",
        application="jobs/bronze/hris_incremental.py",
    )

    silver_employees = SparkSubmitOperator(
        task_id="silver_hris_employees",
        application="jobs/silver/hris_employees.py",
    )

    gold_hr_analytics = SparkSubmitOperator(
        task_id="gold_hr_analytics",
        application="jobs/gold/hr_analytics.py",
    )

    # Dependencies
    [bronze_erp, bronze_hris] >> silver_employees >> gold_hr_analytics
```

---

## 📈 Performance Benchmarks (Target)

| Job Type | Data Volume | Target Duration |
|----------|-------------|----------------|
| Full load (HRIS) | 50K records | < 2 menit |
| Full load (Sales) | 1M records | < 10 menit |
| Daily incremental | 100K events | < 5 menit |
| IoT stream (1K devices) | 1M readings/jam | < 30 detik lag |
| Gold layer aggregation | 10M records | < 15 menit |
| Ad-hoc Trino query | — | < 3 detik (P95) |
