# 15 — Monitoring & Logging
## Enterprise Digital Platform (EDP)

---

## Status Saat Ini

EDP punya stack observability lengkap (metrics, logs, traces) untuk seluruh 16 service Go, ditambahkan bertahap sebagai 3 fase:

1. **Fase 1 — Metrics**: Prometheus + Grafana
2. **Fase 2 — Logging**: JSON structured logs + request ID correlation + Loki/Promtail
3. **Fase 3 — Tracing**: OpenTelemetry + Jaeger

Plus, seperti sebelumnya:
4. **Health endpoints** di setiap service
5. **Audit trail** via audit-service + Kafka

---

## Metrics (Prometheus + Grafana)

Setiap service punya package `internal/metrics` yang expose 2 metric via `GET /metrics` (format Prometheus, plus Go runtime/process collector bawaan):

| Metric | Tipe | Label |
|---|---|---|
| `http_requests_total` | Counter | `method`, `route`, `status` |
| `http_request_duration_seconds` | Histogram | `method`, `route` |

`route` diambil dari pattern mux yang match (`GET /accounts/{id}`), bukan raw URL — supaya UUID di path tidak bikin cardinality meledak.

**Prometheus** (`infra/prometheus/prometheus.yml`, docker-compose only) scrape `host.docker.internal:<port>/metrics` tiap 15 detik untuk semua 16 service — desain ini sengaja seragam untuk 3 mode runtime yang mungkin dipakai (native `go run`, container app-service di docker-compose ini, atau pod K8s dev overlay lewat `kubectl port-forward`), karena ketiganya berakhir bind ke port host yang sama.

**Grafana** (port host 3001, login dev-only `admin`/`admin`) auto-provision datasource Prometheus + dashboard "EDP - Services Overview" (`infra/grafana/provisioning/dashboards/edp-overview.json`) berisi request rate, error rate, p95 latency, goroutines, dan memory per service.

---

## Logging

Setiap service punya package `internal/logging` yang redirect writer stdlib `log` package ke JSON — `log.Printf`/`log.Fatalf` yang sudah ada di seluruh codebase TIDAK perlu diubah sama sekali, teks pesannya otomatis jadi field `msg`.

**Contoh log JSON**:
```json
{"time":"2026-07-21T10:00:00Z","level":"INFO","service":"finance-service","msg":"finance-service listening on :8085"}
```

**Keterbatasan yang disengaja** (bukan bug, trade-off yang didokumentasikan di kode): semua baris level `"INFO"` — writer tidak bisa membedakan `Printf` dari `Fatalf` karena keduanya lewat call stdlib yang sama; leveled logging sungguhan butuh setiap call site ditulis ulang pakai structured logger, di luar scope fase ini.

### Request ID correlation

`internal/requestid` middleware men-generate/menerima header `X-Request-Id` dan log satu access-log line per request bertag ID itu:
```json
{"time":"...","level":"INFO","service":"finance-service","msg":"request_id=abc-123 method=GET path=/accounts"}
```

`api-gateway` men-generate ID kalau caller belum mengirimnya, lalu meneruskannya ke service tujuan dan mengembalikannya ke caller — jadi satu request bisa ditelusuri lintas gateway → service di log stream yang sama.

**Keterbatasan**: ID belum di-thread ke `log.Printf` yang lebih dalam di dalam handler, atau ke panggilan HTTP antar-service (financeclient/warehouseclient) — baris access-log di atas cukup untuk menjawab "apakah request ini benar-benar sampai ke service ini, kapan", tapi belum full distributed correlation (itu baru didapat lewat tracing di bawah).

### Loki + Promtail

`infra/docker-compose.yml` (docker-compose only) punya `loki` (log storage, port 3100) dan `promtail` (ship log SEMUA container di Docker daemon ini ke Loki lewat Docker service discovery). Grafana dapat datasource Loki kedua.

**Keterbatasan**: Promtail cuma menangkap log **container**, bukan proses `go run` native — yang merupakan workflow dev utama di repo ini.

---

## Tracing (OpenTelemetry + Jaeger)

Setiap service punya package `internal/tracing` yang init `TracerProvider` (100%-sampled, dev-only) export span via OTLP/HTTP, dibungkus dengan `otelhttp.NewHandler` di top-level handler-nya — jadi setiap request masuk menghasilkan span otomatis.

**Jaeger** (`jaegertracing/all-in-one`, docker-compose only) jadi collector: UI di port 16686, OTLP/HTTP receiver di port 4318. Proses native `go run` pakai default `OTLP_ENDPOINT=localhost:4318` (reach lewat port host); container app-service di docker-compose pakai `OTLP_ENDPOINT=jaeger:4318` (container-to-container). Grafana dapat datasource Jaeger ketiga.

**Desain**: exporter init gagal bersifat non-fatal (sama seperti best-effort Kafka/MQTT publish di service lain) — kalau Jaeger tidak reachable, span gagal export dengan log warning, service tetap jalan normal.

**Keterbatasan**: in-memory storage saja (trace tidak survive restart container), belum ada production-grade backend (Elasticsearch/Cassandra) untuk Jaeger, dan belum ada di K8s manifests sama sekali (sengaja — konsisten dengan Prometheus/Grafana/Loki yang juga docker-compose only, lihat komentar di `infra/docker-compose.yml`).

---

## Health Endpoints

Setiap service mengekspos `GET /health` yang mengembalikan:

```json
{
  "status": "ok",
  "service": "finance-service"
}
```

Cek semua service sekaligus:
```bash
for port in 8079 8081 8082 8083 8084 8085 8086 8087 8088 8089 8090 8091 8092 8093 8094 8095; do
  curl -s http://localhost:$port/health; echo
done
```

Health endpoint dipakai oleh:
- Kubernetes liveness probe (`httpGet: path: /health`)
- Docker healthcheck
- Manual verifikasi deployment

---

## Audit Trail (via audit-service)

`audit-service` menyimpan semua event bisnis ke tabel `audit_logs` (Postgres) — observability layer bisnis, terpisah dari metrics/logs/traces teknis di atas.

```sql
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID,
    event_type VARCHAR(100) NOT NULL,
    source_service VARCHAR(50) NOT NULL,
    actor_user_id UUID,
    actor_email VARCHAR(255),
    company_id UUID,
    branch_id UUID,
    action VARCHAR(50) NOT NULL,
    entity_type VARCHAR(100) NOT NULL,
    entity_id UUID,
    payload JSONB,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Query audit trail:
```sql
-- Semua aksi user tertentu
SELECT event_type, entity_type, entity_id, occurred_at
FROM audit_logs
WHERE actor_user_id = 'uuid-user'
ORDER BY occurred_at DESC;

-- Semua event untuk satu entitas
SELECT * FROM audit_logs
WHERE entity_id = 'uuid-entity'
ORDER BY occurred_at;
```

`GET /api/audit/logs` tersedia untuk query via API (dengan filter company_id, entity_type, dll). Meng-cover `finance.*`, `hr.*`, `sales.*`, `purchasing.*`, `warehouse.*`, `production.*`, `qc.*`, `asset.*`, `iot.*`, `auth.*`, `company.*`, `rbac.*` — daftar lengkap topic di `backend/services/audit-service/internal/consumer/consumer.go` (`Topics` slice), dijaga sinkron manual dengan tiap `h.events.Publish(...)` call di seluruh service (diverifikasi cross-check penuh, tidak ada topic yang di-publish tapi tidak di-subscribe).

---

## DW Sync Status

`dw-service` menyediakan observability untuk data pipeline:

```bash
curl http://localhost:8095/sync/status
```

```json
[
  {"fact": "finance_journal_lines", "row_count": 1250, "last_synced_at": "2026-07-21T10:00:00Z"},
  {"fact": "sales_order_lines", "row_count": 340, "last_synced_at": "2026-07-21T10:00:00Z"},
  ...
]
```

---

## Roadmap Observability (sisa yang belum dikerjakan)

| Item | Kenapa belum |
|------|--------------|
| **Leveled logging sungguhan** (bukan semua "INFO") | Butuh setiap call site log.Printf/Fatalf ditulis ulang pakai structured logger — di luar scope redirect writer yang ada sekarang |
| **Request ID di setiap log.Printf dalam handler** | Butuh context threading di semua call site, belum dikerjakan |
| **Promtail menangkap proses native `go run`** | Promtail cuma lihat log container Docker; workflow dev utama masih native |
| **Jaeger production-grade backend** | Storage in-memory saja saat ini, tidak ada Elasticsearch/Cassandra backend |
| **Observability tools di K8s manifests** | Sengaja docker-compose only, konsisten di seluruh 4 tool (Prometheus/Grafana/Loki/Jaeger) |

Prioritas saat ini: metrics + logs + traces + audit trail sudah cukup lengkap untuk development dan demo. Item di atas adalah pendalaman untuk kebutuhan production sungguhan.
