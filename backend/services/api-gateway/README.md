# API Gateway

Titik masuk tunggal (single entry point) untuk seluruh client (web/mobile) ke platform. Tanggung jawab:
- Routing request ke microservice terkait (auth, company, rbac, audit, dan modul bisnis lainnya)
- Verifikasi token JWT sebelum meneruskan request (validasi penuh tetap di auth-service)
- Rate limiting & request logging
- Agregasi response lintas service bila diperlukan

## Menjalankan secara lokal
```
go run ./cmd/server
```
Default port: `8080` (lihat `configs/config.yaml` atau env `PORT`).

## Struktur
```
api-gateway/
├── cmd/server/            # entrypoint
├── internal/config/       # konfigurasi via env
├── internal/router/       # routing table & reverse proxy
├── internal/handler/      # handler HTTP tambahan (reserved)
├── internal/middleware/   # auth check, logging, rate limit (reserved)
├── api/                   # kontrak OpenAPI
├── configs/                # config.yaml
└── deployments/            # Dockerfile
```

## Status
Fase 1 — skeleton service. Implementasi reverse proxy & middleware menyusul sesuai `20_Implementation_Guide.md`.
