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
| `/production/bom` | BomPage | Bill of Material |
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
| `/aibi/dashboards` | BIDashboardsPage | Metrik agregat dari 8 service |
| `/aibi/forecasting` | ForecastingPage | Proyeksi tren (linear) dengan chart |
| `/aibi/anomaly` | AnomalyDetectionPage | Deteksi outlier (z-score) |

### Data Warehouse
| Path | Komponen | Fungsi |
|------|----------|--------|
| `/dw/sync-status` | SyncStatusPage | Status ETL per fact table + tombol Sync Now |

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
VITE_API_BASE_URL=http://localhost:8079  (default, diteruskan ke Vite saat build)

apiClient.get('/api/finance/accounts', { params: { company_id, branch_id } })
    └── axios instance dengan base URL = VITE_API_BASE_URL
```

Semua panggilan lewat `api-gateway:8079` dengan prefix `/api/{service}/`.
