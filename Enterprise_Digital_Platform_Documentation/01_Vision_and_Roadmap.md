# Vision & Roadmap


## Prinsip Platform
- Multi Company (company_id pada seluruh data transaksi)
- Multi Branch
- Multi Departement
- Role Based Access Control (RBAC)
- Audit Log
- JWT/OAuth2
- Microservices
- Event Driven (Kafka)
- PostgreSQL + ClickHouse + Redis + MinIO
- Frontend React Js Bootstrap 5
- Backend golang API

## Struktur Multi Company

```
Platform
 ├── Company A
 │    ├── Branch 1
 │    └── Branch 2
 └── Company B
      ├── Branch 1
      └── Branch 2
```

## User Role
| Role | Hak Akses |
|---|---|
| Super Admin | Semua Company |
| Company Admin | Company sendiri |
| Branch Manager | Branch sendiri |
| Finance | Finance |
| HR | HRIS |
| Sales | Sales |
| Purchasing | Purchasing |
| Warehouse | Gudang |
| Production | MES |
| QC | Quality |
| Asset | Asset |
| Auditor | Read Only |
| AI Analyst | AI & BI |


## Ruang Lingkup
Dokumen ini menjelaskan vision & roadmap untuk Enterprise Digital Platform. Berisi tujuan, arsitektur, standar implementasi, rekomendasi teknologi, diagram konseptual, checklist implementasi, dan milestone.

## Checklist
- Desain
- Implementasi
- Pengujian
- Dokumentasi
