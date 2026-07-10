// Backend readiness snapshot (see backend/services vs backend/modules),
// used by DashboardPage's module-status card. The sidebar itself no longer
// reads a static list -- it renders whatever GET /api/rbac/menu-tree
// returns for the logged-in user (see components/layout/Sidebar.jsx).
export const CORE_SERVICES = [
  { key: 'api-gateway', label: 'API Gateway' },
  { key: 'auth', label: 'Auth' },
  { key: 'company', label: 'Company' },
  { key: 'rbac', label: 'RBAC' },
  { key: 'audit', label: 'Audit' },
]

export const BUSINESS_MODULES = [
  { key: 'finance', label: 'Finance' },
  { key: 'hr', label: 'HR' },
  { key: 'sales', label: 'Sales' },
  { key: 'purchasing', label: 'Purchasing' },
  { key: 'warehouse', label: 'Warehouse' },
  { key: 'production', label: 'Production' },
  { key: 'qc', label: 'QC' },
  { key: 'asset', label: 'Asset' },
  { key: 'ai', label: 'AI' },
  { key: 'bi', label: 'BI' },
]
