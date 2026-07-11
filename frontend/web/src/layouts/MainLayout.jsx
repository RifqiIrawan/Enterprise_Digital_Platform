import { useEffect, useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import Sidebar from '../components/layout/Sidebar.jsx'
import Topbar from '../components/layout/Topbar.jsx'
import apiClient from '../services/apiClient.js'
import { getCurrentUser } from '../utils/auth.js'
import { CompanyProvider } from '../store/CompanyContext.jsx'

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

function MainLayout() {
  const [collapsed, setCollapsed] = useState(isMobile)
  const [moduleTree, setModuleTree] = useState(null)
  const [menuError, setMenuError] = useState('')
  const { pathname } = useLocation()

  useEffect(() => {
    const user = getCurrentUser()
    if (!user) return
    apiClient
      .get('/api/rbac/menu-tree', { params: { user_id: user.id } })
      .then(({ data }) => setModuleTree(data))
      .catch(() => setMenuError('Gagal memuat menu dari server.'))
  }, [])

  const title = flattenTitles(moduleTree)[pathname] ?? 'Enterprise Digital Platform'

  return (
    <CompanyProvider>
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
        </div>
      </div>
    </CompanyProvider>
  )
}

export default MainLayout
