# 15 — Monitoring & Logging
## Enterprise Data Center Simulator (EDCS)

---

## 🔭 Overview

EDCS mengimplementasikan **Three Pillars of Observability**: Metrics (Prometheus + Grafana), Logs (ELK Stack), dan Traces (OpenTelemetry + Jaeger/Tempo). Ditambah alerting via **Alertmanager + PagerDuty** dan real-time anomaly detection berbasis ML.

---

## 🏗️ Observability Stack

```
┌──────────────────────────────────────────────────────────────────┐
│                     OBSERVABILITY PLATFORM                       │
│                                                                  │
│  ┌────────────────┐  ┌────────────────┐  ┌───────────────────┐  │
│  │  METRICS       │  │  LOGS          │  │  TRACES           │  │
│  │  Prometheus    │  │  Filebeat      │  │  OpenTelemetry    │  │
│  │  Grafana       │  │  Logstash      │  │  Collector        │  │
│  │  Alertmanager  │  │  Elasticsearch │  │  Jaeger / Tempo   │  │
│  │                │  │  Kibana        │  │  Grafana Tempo    │  │
│  └────────────────┘  └────────────────┘  └───────────────────┘  │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │            UNIFIED DASHBOARD (Grafana)                   │   │
│  │  Correlate metrics ↔ logs ↔ traces dalam satu view       │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │            ALERTING                                      │   │
│  │  Alertmanager → PagerDuty → On-call rotation            │   │
│  │  Alertmanager → Slack → Team channels                   │   │
│  └──────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
```

---

## 📊 METRICS — Prometheus + Grafana

### Prometheus Scrape Config
```yaml
# prometheus/prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: edcs-prod
    environment: production

rule_files:
  - /etc/prometheus/rules/*.yml

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']

scrape_configs:
  # Kubernetes pods (auto-discovery)
  - job_name: 'kubernetes-pods'
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__meta_kubernetes_namespace]
        target_label: namespace
      - source_labels: [__meta_kubernetes_pod_name]
        target_label: pod
      - source_labels: [__meta_kubernetes_pod_label_app]
        target_label: service

  # Kafka metrics (JMX Exporter)
  - job_name: 'kafka'
    static_configs:
      - targets: ['kafka-0:9404', 'kafka-1:9404', 'kafka-2:9404']
    labels:
      cluster: edcs-kafka

  # PostgreSQL (postgres_exporter)
  - job_name: 'postgresql'
    static_configs:
      - targets:
        - 'postgres-hris-exporter:9187'
        - 'postgres-crm-exporter:9187'
        - 'postgres-finance-exporter:9187'

  # Node exporter (hardware metrics)
  - job_name: 'nodes'
    kubernetes_sd_configs:
      - role: node
```

### Alerting Rules
```yaml
# prometheus/rules/business-alerts.yml
groups:
  - name: business_critical
    rules:
      # Service down
      - alert: ServiceDown
        expr: up{job=~"edcs-.*"} == 0
        for: 1m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "Service {{ $labels.service }} is DOWN"
          description: "{{ $labels.service }} in {{ $labels.namespace }} has been down for > 1 minute"
          runbook: "https://wiki.edcs.internal/runbooks/service-down"

      # High error rate
      - alert: HighErrorRate
        expr: |
          sum(rate(http_requests_total{status=~"5.."}[5m])) by (service)
          /
          sum(rate(http_requests_total[5m])) by (service)
          > 0.05
        for: 2m
        labels:
          severity: high
        annotations:
          summary: "High error rate on {{ $labels.service }}: {{ $value | humanizePercentage }}"

      # Kafka consumer lag
      - alert: KafkaConsumerLagHigh
        expr: kafka_consumer_group_lag > 10000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Kafka consumer lag high: {{ $value }} messages behind"

      # Database connection pool exhausted
      - alert: DBConnectionPoolExhausted
        expr: pg_stat_activity_count / pg_settings_max_connections > 0.85
        for: 2m
        labels:
          severity: high
        annotations:
          summary: "PostgreSQL connection pool {{ $value | humanizePercentage }} used"

      # Payroll processing failure
      - alert: PayrollProcessingFailed
        expr: increase(payroll_run_failures_total[1h]) > 0
        labels:
          severity: critical
          team: hr
        annotations:
          summary: "Payroll processing failed — immediate action required"

  - name: infrastructure
    rules:
      - alert: NodeCPUHigh
        expr: 100 - (avg by (node) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100) > 85
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Node {{ $labels.node }} CPU usage: {{ $value }}%"

      - alert: NodeDiskFull
        expr: (node_filesystem_avail_bytes / node_filesystem_size_bytes) < 0.15
        for: 5m
        labels:
          severity: high
        annotations:
          summary: "Disk {{ $labels.mountpoint }} on {{ $labels.node }}: only {{ $value | humanizePercentage }} free"

      - alert: PodOOMKilled
        expr: kube_pod_container_status_last_terminated_reason{reason="OOMKilled"} == 1
        labels:
          severity: high
        annotations:
          summary: "Pod {{ $labels.pod }} OOMKilled — increase memory limit"
```

### Grafana Dashboards
```yaml
# 20+ Pre-built Grafana dashboards

Platform Dashboards:
  - EDCS Overview (Golden Signals per service)
  - Kubernetes Cluster Overview
  - Node Resources (CPU/Mem/Disk per node)
  - Kafka Cluster Health
  - PostgreSQL Performance

Business Dashboards:
  - HRIS Service Metrics (request rate, latency, errors)
  - Payroll Processing Pipeline
  - Sales Order Processing Rate
  - WMS Pick Rate (orders/hour)
  - MES Production Rate (units/hour)
  - Finance Transaction Volume
  - IoT Device Connectivity

Data Platform:
  - Spark Job Duration & Success Rate
  - Kafka Topic Lag per Consumer Group
  - Data Lake Storage Growth
  - dbt Run Status & Duration
  - Airflow DAG Success Rate
```

---

## 📝 LOGS — ELK Stack

### Log Collection (Filebeat DaemonSet)
```yaml
# filebeat/filebeat.yml
filebeat.inputs:
  - type: container
    paths:
      - /var/log/containers/*.log
    processors:
      - add_kubernetes_metadata:
          host: ${NODE_NAME}
          matchers:
            - logs_path:
                logs_path: "/var/log/containers/"
      - decode_json_fields:
          fields: ["message"]
          target: ""
          overwrite_keys: true

output.logstash:
  hosts: ["logstash:5044"]
  loadbalance: true
```

### Logstash Pipeline
```ruby
# logstash/pipeline/edcs.conf
input {
  beats { port => 5044 }
}

filter {
  # Parse structured JSON logs dari services
  if [message] =~ /^\{/ {
    json { source => "message" }
  }

  # Enrichment
  mutate {
    add_field => {
      "[@metadata][index_prefix]" => "edcs-%{[kubernetes][namespace]}"
    }
  }

  # Mask PII
  if [body][email] {
    mutate {
      gsub => ["[body][email]", "@.*", "@***.***"]
    }
  }

  # Parse slow query logs dari PostgreSQL
  if [kubernetes][labels][app] =~ /postgres/ {
    grok {
      match => { "message" => "duration: %{NUMBER:query_duration_ms:float} ms  statement: %{GREEDYDATA:sql_query}" }
    }
    if [query_duration_ms] and [query_duration_ms] > 1000 {
      mutate { add_tag => ["slow_query"] }
    }
  }

  # Business event parsing
  if [event_type] {
    mutate { add_field => { "[@metadata][is_business_event]" => "true" } }
  }
}

output {
  elasticsearch {
    hosts => ["elasticsearch:9200"]
    index => "%{[@metadata][index_prefix]}-%{+YYYY.MM.dd}"
    template_name => "edcs-logs"
    ilm_enabled => true
    ilm_rollover_alias => "edcs-logs"
    ilm_policy => "edcs-log-policy"
  }
}
```

### Elasticsearch ILM Policy
```json
{
  "policy": {
    "phases": {
      "hot": {
        "min_age": "0ms",
        "actions": {
          "rollover": {
            "max_age": "1d",
            "max_size": "50gb"
          }
        }
      },
      "warm": {
        "min_age": "7d",
        "actions": {
          "shrink": { "number_of_shards": 1 },
          "forcemerge": { "max_num_segments": 1 }
        }
      },
      "cold": {
        "min_age": "30d",
        "actions": {
          "freeze": {}
        }
      },
      "delete": {
        "min_age": "90d",
        "actions": {
          "delete": {}
        }
      }
    }
  }
}
```

### Kibana Saved Searches
```
- "All errors last 1h" → level:ERROR AND @timestamp:[now-1h TO now]
- "Slow queries" → tags:slow_query AND query_duration_ms:>1000
- "Failed payments" → event_type:PAYMENT_FAILED
- "Kafka DLQ messages" → kubernetes.labels.app:*DLQ*
- "Auth failures" → event_type:AUTH_FAILED
- "Business events" → @metadata.is_business_event:true
```

---

## 🔍 TRACES — OpenTelemetry + Tempo

### Auto-Instrumentation (Node.js)
```typescript
// src/tracing.ts — load sebelum aplikasi start
import { NodeSDK } from '@opentelemetry/sdk-node';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-grpc';
import { getNodeAutoInstrumentations } from '@opentelemetry/auto-instrumentations-node';

const sdk = new NodeSDK({
  serviceName: process.env.SERVICE_NAME || 'unknown',
  traceExporter: new OTLPTraceExporter({
    url: process.env.OTEL_EXPORTER_OTLP_ENDPOINT || 'http://otel-collector:4317',
  }),
  instrumentations: [
    getNodeAutoInstrumentations({
      '@opentelemetry/instrumentation-http': { enabled: true },
      '@opentelemetry/instrumentation-express': { enabled: true },
      '@opentelemetry/instrumentation-pg': { enabled: true },
      '@opentelemetry/instrumentation-kafkajs': { enabled: true },
      '@opentelemetry/instrumentation-redis': { enabled: true },
    }),
  ],
});

sdk.start();
```

### Custom Business Spans
```typescript
// Tambah business context ke traces
import { trace, context, SpanStatusCode } from '@opentelemetry/api';

const tracer = trace.getTracer('hris-service');

async function processPayroll(payrollRunId: string) {
  const span = tracer.startSpan('payroll.process', {
    attributes: {
      'payroll.run_id': payrollRunId,
      'payroll.period': '2026-06',
      'business.domain': 'HRIS',
    }
  });

  try {
    const result = await calculatePayroll(payrollRunId);
    span.setAttributes({
      'payroll.employee_count': result.employeeCount,
      'payroll.total_amount': result.totalNetAmount,
    });
    span.setStatus({ code: SpanStatusCode.OK });
    return result;
  } catch (err) {
    span.recordException(err);
    span.setStatus({ code: SpanStatusCode.ERROR });
    throw err;
  } finally {
    span.end();
  }
}
```

---

## 🚨 Alertmanager Routing

```yaml
# alertmanager/alertmanager.yml
global:
  resolve_timeout: 5m
  slack_api_url: '$SLACK_WEBHOOK_URL'

route:
  group_by: ['alertname', 'namespace']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 12h
  receiver: 'slack-default'
  routes:
    - match:
        severity: critical
      receiver: 'pagerduty-critical'
      continue: true
    - match:
        severity: critical
      receiver: 'slack-critical'
    - match:
        team: hr
      receiver: 'slack-hr-team'
    - match:
        team: finance
      receiver: 'slack-finance-team'

receivers:
  - name: 'pagerduty-critical'
    pagerduty_configs:
      - service_key: '$PAGERDUTY_SERVICE_KEY'
        description: '{{ range .Alerts }}{{ .Annotations.summary }}{{ end }}'

  - name: 'slack-critical'
    slack_configs:
      - channel: '#edcs-critical-alerts'
        title: '🚨 CRITICAL: {{ .CommonAnnotations.summary }}'
        text: '{{ range .Alerts }}{{ .Annotations.description }}\nRunbook: {{ .Annotations.runbook }}{{ end }}'

  - name: 'slack-default'
    slack_configs:
      - channel: '#edcs-alerts'
        title: '⚠️ {{ .CommonAnnotations.summary }}'
```

---

## 📊 SLO Definitions

| Service | SLI | SLO Target | Error Budget |
|---------|-----|-----------|--------------|
| API Gateway | Availability | 99.9% | 43.8 min/bulan |
| HRIS Service | Latency P95 | < 500ms | 5% requests |
| Payment Processing | Success Rate | 99.95% | 0.05% |
| Kafka Delivery | Message Delivered | 99.99% | 0.01% |
| Data Pipeline | Freshness | < 30 menit | 1 jam/bulan |
| IoT Ingestion | Throughput | > 10K msg/s | 5% degradasi |
