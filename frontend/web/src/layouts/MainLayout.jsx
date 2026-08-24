import { useEffect, useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import Sidebar from '../components/layout/Sidebar.jsx'
import Topbar from '../components/layout/Topbar.jsx'
import Footer from '../components/layout/Footer.jsx'
import apiClient from '../services/apiClient.js'
import { getCurrentUser } from '../utils/auth.js'
import { CompanyProvider, useCompany } from '../store/CompanyContext.jsx'
import { PermissionProvider } from '../store/PermissionContext.jsx'

const isMobile = () => typeof window !== 'undefined' && window.innerWidth < 768

function flattenTitles(moduleTree) {
  const titles = { '/': 'Dashboard' }
  function walk(menu) {
    if (menu.path) titles[menu.path] = menu.name
    menu.children?.forEach(walk)
  }
  moduleTree?.forEach((mod) => mod.menus.forEach(walk))
  return titles
}

// Shell dipisah dari MainLayout supaya bisa memakai useCompany(): menu-tree
// perlu company_id agar override permission per user ikut diperhitungkan, dan
// context-nya baru tersedia DI DALAM CompanyProvider.
function Shell() {
  const [collapsed, setCollapsed] = useState(isMobile)
  const [moduleTree, setModuleTree] = useState(null)
  const [menuError, setMenuError] = useState('')
  const { pathname } = useLocation()
  const { companyId } = useCompany()

  useEffect(() => {
    const user = getCurrentUser()
    if (!user) return
    // Ditunggu sampai company-nya diketahui, tidak dipanggil lebih dulu tanpa
    // company seperti dulu: hak role sekarang di-scope per company, jadi
    // jawaban tanpa company tidak ada artinya -- backend pun menolaknya (400).
    // Efek sampingnya justru bagus: sidebar tidak lagi sempat menampilkan menu
    // gabungan lintas company lalu menyusut begitu company-nya terpilih.
    if (!companyId) return
    apiClient
      .get('/api/rbac/menu-tree', { params: { user_id: user.id, company_id: companyId } })
      .then(({ data }) => setModuleTree(data))
      .catch(() => setMenuError('Gagal memuat menu dari server.'))
  }, [companyId])

  const title = flattenTitles(moduleTree)[pathname] ?? 'Enterprise Digital Platform'

  return (
    <PermissionProvider>
      <div className="edp-shell d-flex">
        {!collapsed && isMobile() && (
          <div className="edp-sidebar-backdrop d-md-none" onClick={() => setCollapsed(true)} />
        )}
        <Sidebar
          collapsed={collapsed}
          onNavigate={() => isMobile() && setCollapsed(true)}
          moduleTree={moduleTree}
          menuError={menuError}
        />
        <div className="flex-grow-1 d-flex flex-column min-vw-0">
          <Topbar title={title} onToggleSidebar={() => setCollapsed((v) => !v)} />
          <main className="edp-content flex-grow-1 p-3 p-md-4">
            <Outlet />
          </main>
          <Footer />
        </div>
      </div>
    </PermissionProvider>
  )
}

function MainLayout() {
  return (
    <CompanyProvider>
      <Shell />
    </CompanyProvider>
  )
}

export default MainLayout
