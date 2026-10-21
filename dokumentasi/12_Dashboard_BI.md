# 12 — Dashboard & Business Intelligence
## Enterprise Digital Platform (EDP)

---

## Overview

BI layer EDP adalah **React SPA** (`frontend/web/`) yang mengkonsumsi semua service Go lewat `api-gateway`. Tidak ada Apache Superset, tidak ada embedded analytics tools, tidak ada Mobile BI terpisah.

---

## Teknologi Frontend

| Komponen | Teknologi |
|----------|-----------|
| Framework | React 18 |
| Build Tool | Vite |
| HTTP Client | Axios (via `apiClient` wrapper) |
| Styling | CSS Modules / inline styles |
| Routing | React Router v6 |
| State | React Context (CompanyContext untuk multi-tenant) |
| Port dev | 3000 (auto-naik kalau occupied) |

---

## Daftar Halaman

### Platform
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/login` | LoginPage | Auth dengan JWT |
| `/dashboard` | DashboardPage | Overview + navigasi |
| `/admin/roles` | RoleListPage, RoleCreatePage | Manajemen role |
| `/admin/roles/:id/permissions` | RolePermissionMatrixPage | Matrix permission per role |
| `/admin/users` | UserRoleAssignmentPage | Assign role ke user |

### Finance
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/finance/chart-of-accounts` | ChartOfAccountsPage | CoA CRUD |
| `/finance/journal` | JournalPage | Journal Entry (DRAFT → POST) |
| `/finance/invoices` | InvoicesPage | Invoice AR/AP |
| `/finance/ar-ap` | ArApPage | Summary AR/AP |

### HR
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/hr/employees` | EmployeesPage | CRUD karyawan |
| `/hr/attendance` | AttendancePage | Log absensi |
| `/hr/payroll` | PayrollPage | Payroll run + posting ke GL |

### Sales
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/sales/customers` | CustomersPage | CRUD customer |
| `/sales/quotations` | QuotationsPage | Quotation lifecycle |
| `/sales/orders` | SalesOrdersPage | SO lifecycle (Confirm→Fulfill→Invoice) |

### Purchasing
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/purchasing/suppliers` | SuppliersPage | CRUD supplier |
| `/purchasing/requisitions` | RequisitionsPage | PR lifecycle |
| `/purchasing/purchase-orders` | PurchaseOrdersPage | PO lifecycle (Confirm→Receive→Invoice) |

### Warehouse
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/warehouse/products` | ProductsPage | CRUD produk (company-wide) |
| `/warehouse/warehouses` | WarehousesPage | CRUD gudang (company-wide) |
| `/warehouse/stock` | StockPage | Saldo stok + mutasi stok |
| `/warehouse/transfers` | StockTransfersPage | Transfer antar gudang |
| `/warehouse/opname` | StockOpnamePage | Stock opname + posting |

### Production, QC, Asset
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/production/bom` | BomPage | Bill of Material: buat dan sunting komponen / formula (persen per batch) |
| `/production/oee` | OeePage | OEE per mesin (Availability × Performance × Quality) dan penyebab downtime dengan kumulatif Pareto |
| `/production/work-orders` | WorkOrdersPage | WO lifecycle |
| `/production/schedule` | ProductionSchedulePage | View WO by date |
| `/qc/standards` | QualityStandardsPage | Standar mutu per produk |
| `/qc/inspections` | QualityInspectionsPage | Inspeksi (PASS/FAIL/PARTIAL otomatis) |
| `/asset/register` | AssetRegisterPage | CRUD aset |
| `/asset/maintenance` | MaintenanceSchedulePage | Jadwal maintenance |

### IoT
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/iot/devices` | DevicesPage | CRUD device + threshold config |
| `/iot/readings` | ReadingsPage | History sensor readings |
| `/iot/alerts` | AlertsPage | Alert management (OPEN→ACK→RESOLVED) |

### AI/BI
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/ai-bi/dashboards` | BIDashboardsPage | Metrik agregat dari beberapa service dan grafik dari dw-service |
| `/ai-bi/forecasting` | ForecastingPage | Proyeksi tren (linear) dengan chart |
| `/ai-bi/anomaly-detection` | AnomalyDetectionPage | Deteksi outlier (z-score) |
| `/ai-bi/predictive-maintenance` | PredictiveMaintenancePage | Pindai aset yang perlu perawatan |
| `/ai-bi/recommendations` | ReorderRecommendationsPage | Rekomendasi pemesanan ulang stok |
| `/ai-bi/chatbot` | ChatbotPage | Chatbot dokumentasi (rag-service): kutipan dan jawaban |
| `/ai-bi/chatbot-queries` | ChatbotQueriesPage | Riwayat pertanyaan chatbot |

### Dashboard per Peran (BI)
Setiap dashboard menampilkan kartu ringkasan dari ai-bi-service dan grafik bulanan dari dw-service. Akses dicek per menu; satu grafik yang gagal dimuat tidak menjatuhkan grafik lain.

| Path | Komponen | Isi |
|------|----------|-----|
| `/bi/executive` | ExecutiveDashboardPage | Revenue vs expense, nilai penjualan, rencana vs realisasi produksi, sebaran nilai KPI |
| `/bi/sales` | SalesDashboardPage | Nilai penjualan, pipeline CRM per stage, penjualan e-commerce, pengiriman selesai vs dibatalkan |
| `/bi/finance` | FinanceDashboardPage | Revenue vs expense, beban gaji per periode, biaya proyek (timesheet yang sudah diposting) |
| `/bi/warehouse` | WarehouseDashboardPage | Stock in vs stock out, belanja per pemasok |
| `/bi/manufacturing` | ManufacturingDashboardPage | Rencana vs realisasi produksi, OEE, Pareto penyebab downtime, mutu, maintenance |
| `/bi/hr` | HrDashboardPage | Hari cuti per jenis, sebaran dan rata-rata nilai KPI per departemen, gaji bersih vs potongan |

### CRM
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/crm/leads` | LeadsPage | Lead; konversi lead yang QUALIFIED menjadi Account + Contact + Opportunity |
| `/crm/accounts` | AccountsPage | Akun pelanggan |
| `/crm/contacts` | ContactsPage | Kontak per akun |
| `/crm/opportunities` | OpportunitiesPage | Pipeline per stage; menang/kalah lewat aksi khusus |
| `/crm/activities` | ActivitiesPage | Aktivitas yang menempel ke entitas CRM lain (referensi polimorfik) |

### Ticketing
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/ticketing/categories` | TicketCategoriesPage | Kategori tiket per company |
| `/ticketing/tickets` | TicketsPage | Tiket; ditutup dan dibuka kembali lewat aksi khusus |
| `/ticketing/comments` | TicketCommentsPage | Komentar tiket |

### E-Commerce, Fleet, Project
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/ecommerce/orders` | OrdersPage | Pesanan online: bayar, kirim (memotong stok; `409` jika stok kurang), terima, batalkan |
| `/fleet/vehicles` | VehiclesPage | Kendaraan |
| `/fleet/drivers` | DriversPage | Pengemudi |
| `/fleet/delivery-orders` | DeliveryOrdersPage | Surat jalan |
| `/project/projects` | ProjectsPage | Proyek |
| `/project/tasks` | TasksPage | Tugas proyek |
| `/project/timesheets` | TimesheetsPage | Timesheet; biaya diposting ke GL |

### Data Warehouse
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/dw/sync-status` | SyncStatusPage | Status ETL per fact table + tombol Sync Now |
| `/dw/data-lake` | DataLakePage | Dataset Gold (revenue vs expense, nilai penjualan per bulan), rekonsiliasi Silver vs ClickHouse, tombol Build Silver + Gold |

---

## Multi-Tenant Context

`CompanyContext.jsx` menyediakan company switcher dan branch selector yang persisten (localStorage). Setiap request API menyertakan `company_id` dan `branch_id` sebagai parameter atau field.

```jsx
// Setiap halaman list pakai hook ini
const { companyId, branchId } = useCompany()

useEffect(() => {
    fetchData({ company_id: companyId, branch_id: branchId })
}, [companyId, branchId])
```

---

## DataTable Component

`components/common/DataTable.jsx` dipakai di semua halaman list — built-in:
- **Search**: filter client-side pada semua kolom
- **Sort**: ascending/descending per kolom
- **Pagination**: configurable rows per page
- **TruncatedText**: kolom panjang dengan hover tooltip

---

## RBAC-Driven Sidebar

Sidebar menu didapat dari `GET /api/rbac/menu-tree` — service return tree menu sesuai permission user yang login. Tidak ada hardcoded menu di frontend. Modul baru otomatis muncul di sidebar begitu RBAC seed-nya ditambahkan.

---

## Koneksi ke Backend

```
VITE_API_BASE_URL=http://localhost:28079  (default, diteruskan ke Vite saat build)

apiClient.get('/api/finance/accounts', { params: { company_id, branch_id } })
    └── axios instance dengan base URL = VITE_API_BASE_URL
```

Semua panggilan lewat `api-gateway:28079` dengan prefix `/api/{service}/`.
