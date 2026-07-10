# Audit Service

Mencatat seluruh perubahan state penting di platform secara event-driven (bukan dipanggil langsung oleh service lain, untuk menjaga loose coupling).

## Alur
1. Service lain (auth, company, rbac, modul bisnis) publish event ke Kafka setiap ada perubahan (create/update/delete) — lihat `infra/kafka/topics.md`.
2. `internal/consumer` men-subscribe topic terkait dan menulis audit record ke penyimpanan.
3. Endpoint HTTP read-only menyediakan akses ke audit log untuk role `Auditor`.

## Tanggung jawab
- Konsumsi event Kafka lintas service, simpan sebagai audit trail (siapa, kapan, company/branch mana, aksi apa, data sebelum/sesudah)
- Log volume tinggi disarankan ke ClickHouse (lihat `05_Data_Warehouse_Architecture.md`), data operasional ringan boleh ke PostgreSQL
- API read-only untuk pencarian/filter audit log

## Menjalankan secara lokal
```
go run ./cmd/server
```
Default port: `8084`. Butuh PostgreSQL/ClickHouse dan Kafka.

## Struktur
```
audit-service/
├── cmd/server/
├── internal/config/
├── internal/handler/       # query API audit log
├── internal/service/
├── internal/repository/
├── internal/model/         # AuditLog
├── internal/consumer/      # Kafka consumer
├── api/
├── migrations/
├── configs/
└── deployments/
```

## Status
Fase 1 — skeleton service.
