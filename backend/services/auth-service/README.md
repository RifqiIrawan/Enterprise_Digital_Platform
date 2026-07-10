# Auth Service

Bertanggung jawab atas autentikasi & otorisasi berbasis JWT/OAuth2:
- Login, logout, refresh token
- Penyimpanan credential (password hash), MFA (opsional, fase berikutnya)
- Penerbitan access token (short-lived) & refresh token (long-lived)
- Publish event ke Kafka (mis. `auth.user.logged_in`) untuk audit-service

Otorisasi berbasis role (RBAC) didelegasikan ke [`rbac-service`](../rbac-service), auth-service hanya menangani identitas.

## Menjalankan secara lokal
```
go run ./cmd/server
```
Default port: `8081`. Butuh PostgreSQL (`auth_service` db), Redis (session/refresh token blacklist), dan Kafka.

## Struktur
```
auth-service/
├── cmd/server/            # entrypoint
├── internal/config/       # konfigurasi via env
├── internal/handler/      # HTTP handler (login, refresh, logout)
├── internal/service/      # business logic
├── internal/repository/   # akses database (Postgres)
├── internal/model/        # struct domain (User, Credential, Token)
├── internal/middleware/   # JWT validation middleware
├── api/                   # kontrak OpenAPI
├── migrations/            # SQL migration
├── configs/                # config.yaml
└── deployments/            # Dockerfile
```

## Status
Fase 1 — skeleton service + skema database (`migrations/001_init.sql`: users, refresh_tokens). Implementasi login/JWT/OAuth2 menyusul sesuai `20_Implementation_Guide.md`.
