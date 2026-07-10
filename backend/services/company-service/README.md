# Company Service

Sumber kebenaran (source of truth) untuk struktur organisasi platform sesuai prinsip Multi Company / Multi Branch / Multi Departement:
```
Platform
 ├── Company A
 │    ├── Branch 1
 │    └── Branch 2
 └── Company B
      ├── Branch 1
      └── Branch 2
```
Setiap Company dan Branch memiliki `company_id` / `branch_id` yang direferensikan oleh seluruh service transaksional lain (Finance, Sales, dst) untuk isolasi data multi-tenant.

## Tanggung jawab
- CRUD Company, Branch, Department
- Validasi hierarki (branch harus dalam company yang valid, dst)
- Publish event ke Kafka saat company/branch dibuat/diubah (mis. `company.created`, `company.branch.created`) agar service lain bisa sinkronisasi cache

## Menjalankan secara lokal
```
go run ./cmd/server
```
Default port: `8082`. Butuh PostgreSQL (`company_service` db).

## Struktur
```
company-service/
├── cmd/server/
├── internal/config/
├── internal/handler/
├── internal/service/
├── internal/repository/
├── internal/model/        # Company, Branch, Department
├── internal/middleware/
├── api/
├── migrations/
├── configs/
└── deployments/
```

## Status
Fase 1 — skeleton service + skema database (`migrations/001_init.sql`: companies, branches, departments). Mengacu ke `04_Database_Design.md` dan `18_ERD_and_Database_Schema.md`. Hak akses per company/branch/department diatur di [`rbac-service`](../rbac-service).
