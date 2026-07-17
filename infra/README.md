# Infra

Infrastruktur lokal & deployment untuk Enterprise Digital Platform.

## PostgreSQL — jalan native, bukan Docker

PostgreSQL sengaja **tidak** ada di `docker-compose.yml`. Di lingkungan dev ini ia
jalan sebagai Windows service native (`postgresql-x64-18`, port 5432) — lebih
stabil daripada lewat Docker Desktop yang di mesin ini pernah macet
(engine pipe error 500) saat memori menipis akibat banyak proyek Docker lain
berjalan bersamaan. Kalau mesin lain tidak punya kendala serupa, silakan
jalankan Postgres lewat Docker seperti biasa — cukup tambahkan kembali service
`postgres` di `docker-compose.yml` mengikuti pola service lain di file ini.

Setup sekali jalan di Postgres native — **seluruh 13 database** (4 dari Fase 1
+ 9 dari Fase 2; `ai-bi-service` tidak punya database sendiri, murni agregasi
HTTP) wajib sudah ada sebelum service manapun (native atau container) bisa
start, karena migrasi jalan otomatis saat startup tapi tidak bisa membuat
database-nya sendiri:
```sql
CREATE ROLE platform LOGIN PASSWORD 'platform' SUPERUSER;
CREATE DATABASE auth_service;
CREATE DATABASE rbac_service;
CREATE DATABASE company_service;
CREATE DATABASE audit_service;
CREATE DATABASE finance_service;
CREATE DATABASE hr_service;
CREATE DATABASE sales_service;
CREATE DATABASE purchasing_service;
CREATE DATABASE warehouse_service;
CREATE DATABASE production_service;
CREATE DATABASE qc_service;
CREATE DATABASE asset_service;
```
Migrasi tiap service (embed FS + tabel `schema_migrations`, aman dijalankan
berkali-kali) jalan otomatis begitu service-nya start — tidak perlu
menjalankan file `migrations/*.sql` secara manual.

## Local development (ClickHouse, Redis, MinIO, Kafka)
```
cp .env.example .env
./scripts/dev-up.ps1      # atau: docker compose up -d
```

Service yang naik:
| Service | Host Port | Keterangan |
|---|---|---|
| PostgreSQL | 5432 | **Native**, bukan Docker — lihat di atas. Satu database per service (`auth_service`, `company_service`, `rbac_service`, `audit_service`, dst) |
| ClickHouse | 8123 (HTTP), 9101 (native) | OLAP / data warehouse, lihat `05_Data_Warehouse_Architecture.md` |
| Redis | 6379 | Cache, session, rate limiting |
| MinIO | 9004 (API), 9003 (Console) | Object storage (S3-compatible). API port sengaja BUKAN 9002 (default sebelumnya) — sesi Data Lake (2026-07-16/17) menemukan itu bentrok dengan container project lain di mesin dev (`smart-parking-mosquitto`, tidak terkait project ini) |
| Kafka | 9092 (host/native clients), internal listener `kafka:29092` (container clients, lihat di bawah) | Event streaming (KRaft mode, tanpa Zookeeper) |
| Kafka UI | 8099 | Dashboard untuk inspeksi topic/consumer (host 8090 dipakai production-service) |
| Mosquitto | 1883 | Broker MQTT untuk IoT Simulator (`backend/modules/iot-service`) — simulator publish, iot-service subscribe untuk ingest. Config di `infra/mosquitto/mosquitto.conf` (anonymous access, dev-only) |
| Prometheus | 9090 | Scrape `/metrics` dari seluruh 16 service Go tiap 15 detik, lihat `infra/prometheus/prometheus.yml`. Target di-scrape lewat `host.docker.internal:<port>` — jalan sama baiknya untuk service yang jalan native (`go run`), sebagai container di compose ini, maupun sebagai pod K8s yang di-`kubectl port-forward` ke port host yang sama |
| Grafana | 3001 (login dev-only `admin`/`admin`) | Dashboard "EDP - Services Overview" ter-provision otomatis (request rate, error rate, p95 latency, goroutines, memory per service) — datasource Prometheus + Loki sudah ter-wire, tidak perlu setup manual. Host port BUKAN 3000 (default Grafana) karena bentrok dengan `frontend` di compose ini |
| Loki | 3100 | Log storage, lihat `infra/loki/loki-config.yml` (single-binary, filesystem-backed, retensi 7 hari, dev-only) |
| Promtail | — (tidak ada host port, cuma internal) | Ship log dari SEMUA container di Docker daemon ini ke Loki lewat Docker service discovery (`infra/promtail/promtail-config.yml`, akses `/var/run/docker.sock` read-only) — **tidak** menangkap log service yang jalan native lewat `go run` (lihat catatan di bawah) |

Matikan semua: `./scripts/dev-down.ps1`

**Observability — metrics + logs SELESAI, belum traces**: Prometheus/Grafana
(metrics) dan Loki/Promtail (logs) di atas menutup dua pilar pertama dari
roadmap "Observability & DevOps". Setiap service mengekspos `/metrics`
(request count + duration histogram, dilabeli per route pattern seperti
`GET /accounts/{id}` bukan URL mentah, supaya label tidak meledak karena
UUID) lewat `internal/metrics` yang di-copy ke tiap service — pola yang sama
dengan `internal/eventbus`/`internal/store`.

Logging: seluruh 16 service memakai `internal/logging` (juga di-copy per
service) yang me-redirect writer package stdlib `log` supaya tiap
`log.Printf`/`log.Fatalf` (isi pesan TIDAK diubah, tetap prosa bebas seperti
sebelumnya) otomatis jadi satu baris JSON (`time`, `level`, `service`,
`msg`) alih-alih format teks polos `2009/11/10 23:00:00 pesan` bawaan.
**Keterbatasan yang disengaja**: semua baris berlevel `"INFO"` — writer-nya
tidak bisa membedakan `log.Printf` dari `log.Fatalf` (keduanya lewat jalur
stdlib yang sama), leveled logging asli butuh setiap call site ditulis ulang
pakai logger terstruktur (di luar scope sesi ini). `api-gateway`
men-generate/meneruskan header `X-Request-Id` (baru jika belum ada dari
caller) ke service yang di-proxy, dan tiap service (lewat
`internal/requestid`, pola middleware yang sama seperti `internal/metrics`)
mencatat satu baris access-log per request ditandai ID itu — cukup untuk
korelasi "request mana menyentuh service mana, kapan" lewat Loki/Grafana.
**Ini TIDAK** menembus ke setiap `log.Printf` individual di dalam handler
(itu butuh context di-thread ke semua call site, sama seperti alasan level
di atas) atau ke pemanggilan HTTP internal service-ke-service lain
(`financeclient`/`warehouseclient`) — request ID cuma menempel di hop
gateway→service pertama.

**Keterbatasan Promtail**: Docker service discovery cuma melihat container
— log dari service yang dijalankan native lewat `go run` (workflow dev
utama project ini, lihat `NEXT_SESSION.md`) tetap cuma tampil di terminal
masing-masing, tidak terkirim ke Loki. Log terpusat baru kelihatan kalau
service dijalankan lewat `docker compose up -d --build`. Lihat log lewat
Grafana **Explore** (bukan dashboard statis — cocok untuk query ad hoc),
pilih datasource "Loki", contoh query `{container=~".+"}` atau
`{container=~".+"} |= "request_id=<id>"` untuk korelasi satu request lintas
service.

Distributed tracing (Jaeger) sengaja belum dikerjakan — menyusul sebagai
pass terpisah kalau dibutuhkan, mengikuti pola "satu pilar sekaligus" yang
sama seperti pengerjaan Data Warehouse bertahap sebelumnya.

Prometheus/Grafana sengaja **cuma ada di `docker-compose.yml`**, tidak
di-deploy sebagai Pod K8s tersendiri — sama seperti Kafka/Redis/ClickHouse/
MinIO/Mosquitto (lihat komentar di `kubernetes/overlays/dev/kustomization.yaml`),
instance docker-compose yang sama sudah bisa scrape service yang jalan
sebagai Pod K8s lewat `kubectl port-forward` ke port host yang sama, jadi
tidak perlu menduplikasi infra ini ke dalam cluster. Deployment K8s untuk
ke-16 service hanya diberi annotation `prometheus.io/scrape`/`port`/`path`
(lihat `kubernetes/base/*.yaml`) sebagai dokumentasi intent, untuk dipakai
kalau nanti ada Prometheus Operator/service-discovery di-cluster.

## Full app stack di Docker (opsional, di atas infra)

`docker-compose.yml` juga bisa menjalankan seluruh 14 service Go + frontend
sebagai container, bukan cuma infra di atas — berguna untuk mendekati
deployment production tanpa perlu 15 terminal `go run`/`npm run dev` manual.
Prasyarat sama seperti local dev: Postgres native harus sudah jalan dengan
ke-13 database di atas sudah dibuat.

```
cd infra
docker compose up -d --build
```

Karena Postgres tetap native (bukan container), semua service app
terhubung ke sana lewat hostname khusus Docker Desktop
`host.docker.internal` (bukan `localhost`, yang di dalam container merujuk
ke container itu sendiri) — sudah di-wire di `docker-compose.yml` lewat
env var `DATABASE_URL` + `extra_hosts`. Kafka juga punya listener kedua
(`kafka:29092`, internal-only) khusus untuk container, terpisah dari
listener `localhost:9092` yang dipakai proses native — tanpa ini, metadata
response Kafka akan mengarahkan container client untuk reconnect ke
"localhost" versi dirinya sendiri dan produce/consume diam-diam gagal.

Port host yang dipetakan sama persis dengan menjalankan tiap service native
(lihat tabel port di `NEXT_SESSION.md` bagian "Cara Menjalankan") — jadi
frontend tetap diakses di `http://localhost:3000` dan API gateway di
`http://localhost:8079`, baik service-nya jalan native maupun sebagai
container.

Rebuild image setelah ganti kode: `docker compose build <nama-service>` lalu
`docker compose up -d <nama-service>`. Mematikan seluruhnya (app + infra):
`docker compose down`.

**Penting**: `docker-compose.yml` ini murni untuk dev lokal satu mesin (asumsi
`host.docker.internal`, container network yang sama). Untuk deployment
staging/prod sungguhan (topologi berbeda, tanpa `host.docker.internal`), lihat
`environments/` di bawah — bukan file compose ini.

## Environment config staging/prod

`environments/staging/` dan `environments/production/` berisi template env
var (`*.env.example`) untuk tiap 14 service + frontend, siap diisi kalau
infrastruktur staging/prod sungguhan sudah ada — lihat
`environments/README.md` untuk penjelasan lengkap & alasan kenapa ini
terpisah dari `docker-compose.yml`. `auth-service` dan `api-gateway` menolak
start (`log.Fatalf`) kalau `APP_ENV` bukan `development` tapi `JWT_SECRET`
masih default `change-me` — proteksi supaya kesalahan config seperti ini
tidak lolos ke deployment sungguhan.

## Kubernetes (Kustomize)

`kubernetes/base/` + `kubernetes/overlays/{dev,staging,prod}/` — manifest
plain (Deployment/Service/ConfigMap/Secret/Ingress via Kustomize, bukan
Helm). **`overlays/dev` benar-benar bisa dipakai hari ini** terhadap cluster
lokal (Docker Desktop Kubernetes, dites nyata sesi ini — 15 pod naik, semua
`1/1 Running`, health check + panggilan lintas-service lewat K8s Service DNS
sukses, lihat `kubernetes/overlays/dev/README.md` untuk cara pakai).
`overlays/staging`/`overlays/prod` sengaja masih kerangka tipis (namespace +
replica count saja) — belum ada infrastruktur staging/prod sungguhan untuk
diisi nilai aslinya (lihat README masing-masing untuk daftar lengkap yang
masih kurang).

```
kubectl apply -k infra/kubernetes/overlays/dev
kubectl -n edp-dev get pods -w
```

## Struktur
```
infra/
├── docker-compose.yml       # infra (Kafka/Redis/MinIO/ClickHouse) + seluruh app stack lokal (opsional)
├── environments/            # template env var staging/prod (*.env.example per service)
├── kafka/topics.md          # konvensi penamaan topic
├── kubernetes/
│   ├── base/                 # Deployment+Service+ConfigMap+Ingress per service (Kustomize)
│   └── overlays/
│       ├── dev/               # lengkap & bisa dipakai (host.docker.internal, image lokal, jwt-secret dev)
│       ├── staging/            # kerangka tipis, belum bisa dipakai
│       └── prod/                # kerangka tipis, belum bisa dipakai
└── scripts/                 # dev-up.ps1, dev-down.ps1

# Dockerfile tiap service ada di direktorinya sendiri, bukan di sini:
backend/services/<service>/deployments/Dockerfile
backend/modules/<module>/deployments/Dockerfile
frontend/web/Dockerfile
```

## Status
Fase 1 — docker-compose untuk local dev infra SELESAI. Dockerfile + docker-compose
untuk full app stack (14 service Go + frontend) SELESAI. Template environment
config staging/prod (`environments/`) + guard `JWT_SECRET` SELESAI (tapi belum
ada infrastruktur staging/prod sungguhan untuk diisi ke templatenya). Manifest
Kubernetes (Kustomize, `base/` + `overlays/dev` lengkap & teruji, `staging`/`prod`
kerangka tipis) SELESAI. CI/CD belum dikerjakan (butuh git remote dulu supaya
bisa diverifikasi jalan) — satu-satunya item production-readiness yang tersisa
dari 4 opsi awal.
