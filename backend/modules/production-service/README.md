# Production Service / MES

Modul produksi: perencanaan (Fase 2) dan pelaksanaan di lantai produksi (Fase 3).
Role terkait: `Production`. Event dipublikasikan ke Kafka dengan prefix `production.*`.

## Dua lapisan yang sengaja dipisah

| Lapisan | Tabel | Menjawab | Menyentuh stok? |
|---|---|---|---|
| Perencanaan | `bill_of_materials`, `bom_lines`, `work_orders`, `work_order_lines` | apa yang mau dibuat, dari komponen apa, ke gudang mana | **Ya** — hanya saat Work Order COMPLETED |
| Pelaksanaan (MES) | `machines`, `shifts`, `production_runs`, `downtime_logs` | mesin mana, shift kapan, berapa yang jadi, berapa lama berhenti | Tidak |

Pemisahan ini yang membuat stok cuma bergerak di satu tempat. Production run
mencatat kenyataan di lantai produksi; Work Order yang menutup dan memutasi stok.

Jahitannya ada di `completeWorkOrder`: **selama sebuah Work Order tidak punya
production run**, perilakunya persis seperti sebelum MES ada — `quantity_produced`
diketik saat menyelesaikan WO. **Begitu ada run**, run yang jadi sumber kebenaran:
WO tidak bisa diselesaikan selagi ada run yang belum ditutup, dan
`quantity_produced` harus sama dengan jumlah `quantity_good` seluruh run-nya
(ditolak, bukan ditimpa diam-diam — kalau berbeda, salah satunya salah dan itu
harus diketahui orangnya). Unit reject tidak ikut masuk gudang.

## OEE

```
Availability = run time / planned time          run time  = planned - downtime
Performance  = (cycle time ideal × total unit) / run time
Quality      = unit bagus / total unit
OEE          = Availability × Performance × Quality
```

- **planned time** = waktu produktif shift (jam kerja − istirahat), di-*snapshot*
  ke `production_runs.planned_minutes` saat run dibuat, supaya jam shift yang
  diubah bulan depan tidak menulis ulang OEE bulan lalu.
- **cycle time ideal** milik mesin (`machines.ideal_cycle_time_minutes`), jadi
  agregat lintas mesin menjumlahkan menit ideal per mesin lebih dulu — bukan
  memakai satu angka untuk semua.
- Performance di atas 100% dipotong ke 100% dan ditandai `performance_capped`:
  itu tanda cycle time ideal disetel terlalu lambat atau downtime tidak dicatat,
  bukan mesin yang melampaui rancangannya.
- Hanya run `CLOSED` yang ikut dihitung.

## Endpoint

Perencanaan:

| Method | Path | Catatan |
|---|---|---|
| GET/POST | `/boms`, `/boms/{id}` (GET/PUT) | Bill of Material + komponennya |
| GET/POST | `/work-orders`, `/work-orders/{id}` | `work_order_lines` di-snapshot dari BOM saat dibuat |
| POST | `/work-orders/{id}/start` | DRAFT → IN_PROGRESS |
| POST | `/work-orders/{id}/complete` | IN_PROGRESS → COMPLETED; memutasi stok di warehouse-service |

MES (Fase 3):

| Method | Path | Catatan |
|---|---|---|
| GET/POST/PUT | `/machines`, `/machines/{id}` | status ACTIVE/MAINTENANCE/INACTIVE; tidak bisa ditarik dari ACTIVE selagi ada run OPEN |
| GET/POST/PUT | `/shifts`, `/shifts/{id}` | jam "HH:MM", boleh melewati tengah malam; `planned_minutes` dihitung server |
| GET/POST | `/production-runs`, `/production-runs/{id}` | filter: `work_order_id`, `machine_id`, `status`, `from`, `to` |
| POST | `/production-runs/{id}/close` | mengunci `quantity_good`/`quantity_reject`, mengembalikan OEE run itu |
| POST/DELETE | `/production-runs/{id}/downtime[/{downtimeId}]` | hanya selagi run OPEN; total downtime tidak boleh melebihi waktu shift |
| GET | `/oee` | ringkasan per mesin + keseluruhan + Pareto penyebab downtime |

Aturan yang dijaga database, bukan hanya handler: satu mesin hanya boleh punya
satu run `OPEN` (unique index parsial), dan run `OPEN` tidak boleh punya angka
hasil sementara run `CLOSED` wajib punya (CHECK constraint).

## Tes

```
go test ./...
```

Butuh Postgres lokal (`postgres://platform:platform@localhost:5432`); tanpa itu
seluruh test di-skip, bukan gagal. Database `production_service_test` dibuat dan
dimigrasikan otomatis.
