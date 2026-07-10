# RBAC Service

Mengelola Role Based Access Control lintas platform, mengacu ke tabel User Role pada `01_Vision_and_Roadmap.md`:

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

## Tanggung jawab
- CRUD Module, Menu, Role
- Penugasan role ke user dengan scope company / branch / department (`user_roles`)
- Override permission per user, per menu, per scope (`user_menu_permission_overrides`)
- Endpoint otorisasi yang dipanggil service lain (atau via API Gateway middleware) untuk cek permission

## Model akses granular

Akses diatur pada level **menu** (bukan langsung ke tabel/endpoint), dengan 6 aksi per menu: `can_view`, `can_create`, `can_update`, `can_delete`, `can_approve`, `can_export`. Kombinasi ini yang membedakan role/​user "hanya bisa lihat" (`can_view` saja) vs "akses penuh" (semua `true`).

Dua lapis permission:
1. **`role_menu_permissions`** — default akses per role, per menu. Role bisa berupa template global (`company_id` NULL, mis. Super Admin, Auditor) atau custom milik satu company.
2. **`user_menu_permission_overrides`** — override eksplisit untuk satu user, pada scope `company_id` + `branch_id` (opsional) + `department_id` (opsional) + `menu_id` tertentu. Kalau ada, ini menang atas hasil role.

Penugasan role ke user (`user_roles`) juga di-scope ke company/branch/department, sehingga **satu user bisa punya role & akses berbeda di company/branch/department yang berbeda** — sesuai kebutuhan "akses per company, per menu, per departement, dan tiap user bisa berbeda".

### Lintas company & lintas departemen

`user_roles` tidak dibatasi satu baris per user — satu user boleh punya banyak baris `user_roles`, masing-masing dengan `company_id` / `branch_id` / `department_id` berbeda. Artinya:
- **Lintas departemen**: isi `department_id = NULL` pada satu baris `user_roles` agar role berlaku di semua departemen dalam company/branch itu, atau insert beberapa baris untuk departemen spesifik yang berbeda-beda.
- **Lintas company**: insert beberapa baris `user_roles` dengan `company_id` berbeda untuk user yang sama (mis. staf Finance yang menangani Company A & Company B). Role yang dipakai sebaiknya role *template global* (`roles.company_id IS NULL`) agar bisa dipasang di company manapun; role *custom* (`roles.company_id` terisi) divalidasi di application layer agar hanya dipasang pada company pemiliknya.
- **Super Admin**: `users.is_super_admin = true` (di auth-service) memberi akses ke semua company tanpa perlu baris `user_roles` sama sekali.

Semua ini murni data-driven — tidak ada batasan skema, hanya soal baris apa yang di-insert ke `user_roles` dan `user_menu_permission_overrides`.

Algoritma resolusi akses efektif (lihat `internal/service/permission.go`):
```
1. Jika ada override yang cocok persis (user, company, branch, department, menu) -> pakai override.
2. Jika tidak ada override -> gabungkan (OR per kolom) role_menu_permissions dari
   seluruh role yang dimiliki user pada scope tsb (union dari semua baris user_roles
   yang relevan, termasuk lintas company/branch/department bila ada).
3. Jika tidak ada role/override sama sekali -> tidak ada akses.
```

Skema & seed:
- `migrations/001_init.sql` — DDL (modules, menus, roles, role_menu_permissions, user_roles, user_menu_permission_overrides)
- `migrations/002_seed.sql` — modul, menu modul `core`, 13 role bawaan sesuai tabel User Role di atas, permission Super Admin (full) & Auditor (view-only)
- `migrations/003_seed_business_menus.sql` — contoh menu untuk modul Finance/HR/Sales/Purchasing/Warehouse/Production/QC/Asset/AI&BI
- `migrations/004_seed_role_permissions.sql` — default permission 11 role sisanya: Company Admin & Branch Manager (full access lintas modul bisnis, beda perlakuan di menu administrasi core), role fungsional (Finance, HR, Sales, dst) full access hanya ke menu modul miliknya sendiri

## Menjalankan secara lokal
```
go run ./cmd/server
```
Default port: `8083`. Butuh PostgreSQL (`rbac_service` db).

## Struktur
```
rbac-service/
├── cmd/server/
├── internal/config/
├── internal/handler/
├── internal/service/       # permission.go: resolusi akses efektif
├── internal/repository/
├── internal/model/         # Module, Menu, Role, RoleMenuPermission, UserRole, UserMenuPermissionOverride
├── internal/middleware/
├── api/
├── migrations/             # 001_init, 002_seed, 003_seed_business_menus, 004_seed_role_permissions
├── configs/
└── deployments/
```

## Status
Fase 1 — skeleton service + skema database akses granular. Handler/repository (implementasi CRUD & query) menyusul sesuai `20_Implementation_Guide.md`.
