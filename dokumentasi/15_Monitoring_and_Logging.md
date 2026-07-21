# 15 — Monitoring & Logging
## Enterprise Digital Platform (EDP)

---

## Status Saat Ini

EDP belum memiliki stack observability khusus (Prometheus, ELK, OpenTelemetry, Grafana). Monitoring dilakukan melalui:

1. **Structured logging** via Go standard library (`log.Printf`)
2. **Health endpoints** di setiap service
3. **Audit trail** via audit-service + Kafka

---

## Logging

Semua service menggunakan `log.Printf` langsung (Go standard library). Log ditulis ke stdout/stderr. Format tidak terstruktur (plain text), bukan JSON.

**Contoh log yang ada**:
```
2026/07/21 10:00:00 finance-service listening on :8085
2026/07/21 10:00:01 audit-service: failed to decode event from topic sales.order.fulfilled: ...
2026/07/21 10:00:05 consumer[iot.alert.triggered]: connected, first message received (offset 12)
2026/07/21 10:00:10 dw-service: synced 15 rows into fact_finance_journal_lines
2026/07/21 10:00:10 dw-service: datalake write for sales_order_lines failed (ClickHouse sync still succeeded): ...
```

### Level Log yang Dipakai

| Pattern | Kapan |
|---------|-------|
| `log.Printf(...)` | Info operasional normal |
| `log.Printf("error: ...")` | Error yang tidak fatal (service tetap jalan) |
| `log.Fatalf(...)` | Error fatal di startup (koneksi DB gagal, config invalid) |

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

`audit-service` menyimpan semua event bisnis ke tabel `audit_logs` (Postgres). Ini adalah observability layer yang sesungguhnya ada dan berfungsi:

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

`GET /api/audit/logs` tersedia untuk query via API (dengan filter company_id, entity_type, dll).

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

## Roadmap Observability

Yang bisa ditambahkan kalau dibutuhkan:

| Tool | Fungsi | Effort |
|------|--------|--------|
| **Prometheus** | Metrics (request rate, latency, error rate) | Sedang — tambah `/metrics` endpoint + scrape config |
| **Grafana** | Dashboard untuk Prometheus metrics | Sedang — konfigurasi dashboard |
| **Structured Logging (zerolog/zap)** | JSON logs untuk log aggregation | Kecil — replace `log.Printf` |
| **OpenTelemetry** | Distributed tracing lintas service | Besar — instrumentasi semua HTTP client/server |
| **ELK Stack** | Centralized log management | Besar — infra + log shipper |

Prioritas saat ini: sistem sudah berfungsi dengan baik tanpa full observability stack. Health endpoints + audit logs + DW sync status cukup untuk development dan demo.
