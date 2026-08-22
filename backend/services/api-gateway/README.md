# API Gateway

Titik masuk tunggal (single entry point) untuk seluruh client web ke platform. Tanggung jawab:

- **Routing** request `/api/<modul>/...` ke 20 microservice di belakangnya (reverse proxy, prefix dipotong sebelum diteruskan).
- **Autentikasi** — memverifikasi JWT, lalu menerjemahkannya jadi header identitas (`X-User-Id`, `X-User-Email`, `X-Is-Super-Admin`) yang dipakai service tujuan. Hanya `POST /api/auth/login` dan `POST /api/auth/refresh` yang lolos tanpa token.
- **Penegakan hak akses** — memeriksa hak per endpoint SEBELUM meneruskan request (lihat di bawah).
- **Request id & observability** — `X-Request-Id` dibuat/diteruskan/dikembalikan supaya satu request bisa dilacak lintas service, plus metrics Prometheus di `/metrics` dan tracing OTLP.
- **CORS** untuk frontend.

## Penegakan hak akses (`internal/authz`)

Gateway adalah satu-satunya jalan masuk dari browser, jadi di sinilah hak akses ditegakkan — bukan disalin ke 20 service.

- `policy.go` memetakan setiap endpoint ke **menu + aksi** yang dibutuhkannya (mis. `POST /api/finance/invoices/{id}/post` → menu `/finance/invoices`, aksi `approve`). Endpoint yang **tidak ada** di tabel itu **ditolak**, bukan diteruskan diam-diam.
- `client.go` menanyakan hak efektif user ke rbac-service (`GET /access?user_id=&company_id=`) dan menyimpannya sebentar (TTL 30 detik; kalau rbac-service mati, jawaban lama masih dipakai sampai 5 menit, setelah itu request dijawab **503** — bukan diloloskan).
- `enforcer.go` menentukan company yang dituju request: query `company_id` → field `company_id` di body JSON → header `X-Company-Id`. Yang disebut request sendiri selalu menang atas header, supaya hak di satu company tidak bisa dipakai untuk menyentuh data company lain.

Super admin melewati pemeriksaan ini sepenuhnya. Endpoint yang ditandai internal (mis. `POST /api/warehouse/stock-movements/batch`, yang dipanggil service lain secara **langsung**, bukan lewat gateway) ditolak untuk semua pemanggil lewat gateway, super admin termasuk.

`TestPolicyCoversEveryRegisteredRoute` membaca route yang benar-benar terdaftar di source seluruh service dan gagal kalau ada yang belum punya kebijakan — jadi endpoint baru tidak bisa lolos tanpa keputusan hak akses.

**Yang TIDAK dijaga di sini**: service modul tidak memeriksa apa pun sendiri. Siapa pun yang bisa menghubungi port service secara langsung tetap bisa memanggilnya; di deployment compose/K8s hanya gateway yang di-expose keluar.

## Menjalankan secara lokal

```
go run ./cmd/server
```

Default port: `8079` (env `PORT`). Env lain yang perlu diketahui:

| Env | Default | Keterangan |
|---|---|---|
| `JWT_SECRET` | `change-me` | harus sama dengan auth-service; wajib diisi eksplisit kalau `APP_ENV` bukan `development` |
| `RBAC_SERVICE_URL` | `http://localhost:8083` | sumber hak akses yang ditegakkan gateway |
| `AUTHZ_ENFORCE` | `true` | jalan keluar darurat; `false` mengembalikan perilaku lama (token valid = boleh apa saja) |
| `AUTHZ_CACHE_TTL` | `30s` | selama ini hak yang sudah diambil dipakai apa adanya — juga jeda maksimum berlakunya pencabutan hak |
| `AUTHZ_STALE_GRACE` | `5m` | seberapa lama jawaban lama boleh dipakai saat rbac-service tidak bisa dihubungi |
| `CORS_ALLOWED_ORIGIN` | `http://localhost:3000` | origin frontend (localhost port berapa pun selalu diizinkan untuk dev) |

`*_SERVICE_URL` untuk 20 service lainnya mengikuti pola yang sama — lihat `internal/config/config.go`.

## Struktur

```
api-gateway/
├── cmd/server/            # entrypoint
├── internal/authz/        # tabel kebijakan hak akses + client & cache ke rbac-service
├── internal/config/       # konfigurasi via env
├── internal/gateway/      # routing table, reverse proxy, autentikasi, CORS
├── internal/logging/      # log terstruktur
├── internal/metrics/      # Prometheus
├── internal/tracing/      # OTLP
├── api/                   # kontrak OpenAPI
├── configs/               # config.yaml
└── deployments/           # Dockerfile
```
