# Kafka Topic Convention

Format nama topic: `<domain>.<entity>.<action>`

Contoh topic inti (Fase 1):
| Topic | Publisher | Consumer | Keterangan |
|---|---|---|---|
| `auth.user.registered` | auth-service | audit-service | User baru terdaftar |
| `auth.user.logged_in` | auth-service | audit-service | Login berhasil |
| `company.company.created` | company-service | audit-service, rbac-service | Company baru dibuat |
| `company.branch.created` | company-service | audit-service | Branch baru dibuat |
| `rbac.role.assigned` | rbac-service | audit-service | Role diberikan ke user |

Topic modul bisnis (Fase 2, mengikuti `09_Kafka_Streaming.md`): `finance.*`, `sales.*`, `purchasing.*`, `warehouse.*`, `production.*`, `qc.*`, `asset.*`.

## Konvensi
- Setiap event membawa `company_id` (dan `branch_id` bila relevan) di payload untuk mendukung multi-tenant.
- Payload berisi minimal: `event_id`, `occurred_at`, `actor_user_id`, `company_id`, `branch_id`, `payload`.
- audit-service subscribe seluruh topic (wildcard per domain) untuk membangun audit trail terpusat.
