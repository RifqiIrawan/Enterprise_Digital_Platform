import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import MainLayout from './layouts/MainLayout.jsx'
import LoginPage from './pages/auth/LoginPage.jsx'
import DashboardPage from './pages/dashboard/DashboardPage.jsx'
import PlaceholderPage from './pages/PlaceholderPage.jsx'
import RoleListPage from './pages/admin/roles/RoleListPage.jsx'
import RoleCreatePage from './pages/admin/roles/RoleCreatePage.jsx'
import RolePermissionMatrixPage from './pages/admin/roles/RolePermissionMatrixPage.jsx'
import UserRoleAssignmentPage from './pages/admin/users/UserRoleAssignmentPage.jsx'
import ChartOfAccountsPage from './pages/finance/ChartOfAccountsPage.jsx'
import JournalPage from './pages/finance/JournalPage.jsx'
import InvoicesPage from './pages/finance/InvoicesPage.jsx'
import ArApPage from './pages/finance/ArApPage.jsx'
import EmployeesPage from './pages/hr/EmployeesPage.jsx'
import AttendancePage from './pages/hr/AttendancePage.jsx'
import PayrollPage from './pages/hr/PayrollPage.jsx'
import CustomersPage from './pages/sales/CustomersPage.jsx'
import QuotationsPage from './pages/sales/QuotationsPage.jsx'
import SalesOrdersPage from './pages/sales/SalesOrdersPage.jsx'
import SuppliersPage from './pages/purchasing/SuppliersPage.jsx'
import RequisitionsPage from './pages/purchasing/RequisitionsPage.jsx'
import PurchaseOrdersPage from './pages/purchasing/PurchaseOrdersPage.jsx'
import ProductsPage from './pages/warehouse/ProductsPage.jsx'
import WarehousesPage from './pages/warehouse/WarehousesPage.jsx'
import StockPage from './pages/warehouse/StockPage.jsx'
import StockTransfersPage from './pages/warehouse/StockTransfersPage.jsx'
import StockOpnamePage from './pages/warehouse/StockOpnamePage.jsx'
import BomPage from './pages/production/BomPage.jsx'
import WorkOrdersPage from './pages/production/WorkOrdersPage.jsx'
import ProductionSchedulePage from './pages/production/ProductionSchedulePage.jsx'
import QualityStandardsPage from './pages/qc/QualityStandardsPage.jsx'
import QualityInspectionsPage from './pages/qc/QualityInspectionsPage.jsx'
import AssetRegisterPage from './pages/asset/AssetRegisterPage.jsx'
import MaintenanceSchedulePage from './pages/asset/MaintenanceSchedulePage.jsx'
import BIDashboardsPage from './pages/aibi/BIDashboardsPage.jsx'
import { isAuthenticated } from './utils/auth.js'

function RequireAuth({ children }) {
  const location = useLocation()
  if (!isAuthenticated()) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return children
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <RequireAuth>
            <MainLayout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<DashboardPage />} />
        <Route path="/admin/roles" element={<RoleListPage />} />
        <Route path="/admin/roles/new" element={<RoleCreatePage />} />
        <Route path="/admin/roles/:roleId/permissions" element={<RolePermissionMatrixPage />} />
        <Route path="/admin/users" element={<UserRoleAssignmentPage />} />
        <Route path="/finance/accounts" element={<ChartOfAccountsPage />} />
        <Route path="/finance/journal" element={<JournalPage />} />
        <Route path="/finance/invoices" element={<InvoicesPage />} />
        <Route path="/finance/ar-ap" element={<ArApPage />} />
        <Route path="/hr/employees" element={<EmployeesPage />} />
        <Route path="/hr/attendance" element={<AttendancePage />} />
        <Route path="/hr/payroll" element={<PayrollPage />} />
        <Route path="/sales/customers" element={<CustomersPage />} />
        <Route path="/sales/quotations" element={<QuotationsPage />} />
        <Route path="/sales/orders" element={<SalesOrdersPage />} />
        <Route path="/purchasing/suppliers" element={<SuppliersPage />} />
        <Route path="/purchasing/requisitions" element={<RequisitionsPage />} />
        <Route path="/purchasing/orders" element={<PurchaseOrdersPage />} />
        <Route path="/warehouse/products" element={<ProductsPage />} />
        <Route path="/warehouse/warehouses" element={<WarehousesPage />} />
        <Route path="/warehouse/stock" element={<StockPage />} />
        <Route path="/warehouse/transfers" element={<StockTransfersPage />} />
        <Route path="/warehouse/opname" element={<StockOpnamePage />} />
        <Route path="/production/bom" element={<BomPage />} />
        <Route path="/production/work-orders" element={<WorkOrdersPage />} />
        <Route path="/production/schedule" element={<ProductionSchedulePage />} />
        <Route path="/qc/standards" element={<QualityStandardsPage />} />
        <Route path="/qc/inspections" element={<QualityInspectionsPage />} />
        <Route path="/asset/register" element={<AssetRegisterPage />} />
        <Route path="/asset/maintenance" element={<MaintenanceSchedulePage />} />
        <Route path="/ai-bi/dashboards" element={<BIDashboardsPage />} />
        <Route path="*" element={<PlaceholderPage />} />
      </Route>
    </Routes>
  )
}

export default App
