# HR Service / HRIS

Status: **Fase 2 — implemented.**

Lingkup: data karyawan, absensi harian, dan payroll dasar (basic salary
pro-rata + allowance tetap, potongan PPh21 progresif disederhanakan +
BPJS Kesehatan/JHT/JP porsi karyawan), dengan posting payroll ke General
Ledger finance-service.

Mengikuti struktur yang sama dengan `finance-service` (lihat
`internal/httpapi`, `internal/store`, `internal/eventbus`, `migrations/`).
Publish event Kafka `hr.*`:
- `hr.employee.created`, `hr.employee.updated`
- `hr.attendance.created`, `hr.attendance.updated`
- `hr.payroll.processed`, `hr.payroll.posted`

## Endpoints

- `GET/POST /employees`, `GET/PUT /employees/{id}`
- `GET/POST /attendance`, `PUT /attendance/{id}`
- `GET/POST /payroll-runs`, `GET /payroll-runs/{id}`, `POST /payroll-runs/{id}/post`

`POST /payroll-runs` menghitung payroll untuk semua karyawan `ACTIVE` di
sebuah `company_id` + `period` (YYYY-MM), menyimpan `payroll_run` berstatus
`DRAFT`. `POST /payroll-runs/{id}/post` mem-posting run ke GL finance-service
lewat `internal/financeclient` (panggilan HTTP langsung ke `FINANCE_SERVICE_URL`,
bukan lewat api-gateway), butuh minimal `expense_account_id` dan
`salary_payable_account_id` (plus `tax_payable_account_id`/`bpjs_payable_account_id`
kalau ada potongan PPh21/BPJS).

Simplifikasi yang disengaja untuk Fase 2 (konsisten dengan pola finance-service
yang memakai `partner_name` bebas untuk invoice): `department` dan `job_title`
adalah teks bebas di `employees`, bukan master table terpisah. Struktur gaji
(`basic_salary`, `monthly_allowance`, `ptkp_status`) disimpan langsung di baris
employee, bukan tabel `salary_components` + `employee_salaries` ber-effective-date.

Perhitungan PPh21 memakai tarif progresif standar (bukan metode TER penuh)
setelah dikurangi biaya jabatan (5%, maks Rp 6.000.000/tahun) dan PTKP.
