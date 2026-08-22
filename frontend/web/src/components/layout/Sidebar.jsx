import { useEffect, useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import Logo from '../common/Logo.jsx'
import { findModuleIdForPath, moduleContainsPath } from '../../utils/menuTree.js'

function MenuItem({ menu, onNavigate, depth = 0 }) {
  return (
    <li>
      <NavLink
        to={menu.path || '#'}
        end
        onClick={onNavigate}
        className={({ isActive }) =>
          `edp-nav-link nav-link d-flex align-items-center gap-2 px-2 py-2 ${isActive ? 'active' : ''}`
        }
        style={{ paddingLeft: `${0.5 + depth * 1}rem` }}
      >
        <i className={`bi ${menu.icon || 'bi-app-indicator'}`} />
        {menu.name}
      </NavLink>
      {menu.children?.length > 0 && (
        <ul className="nav nav-pills flex-column gap-1">
          {menu.children.map((child) => (
            <MenuItem key={child.id} menu={child} onNavigate={onNavigate} depth={depth + 1} />
          ))}
        </ul>
      )}
    </li>
  )
}

function Sidebar({ collapsed, onNavigate, moduleTree, menuError }) {
  const { pathname } = useLocation()
  // Satu grup terbuka pada satu waktu. Dengan 16 modul, membiarkan semuanya
  // terbuka membuat sidebar jadi daftar panjang yang harus di-scroll terus --
  // itu justru masalah yang mau diselesaikan accordion.
  const [openModuleId, setOpenModuleId] = useState(null)

  // Buka grup yang memuat halaman aktif. Efeknya bergantung pada pathname saja
  // (plus moduleTree yang datangnya async), jadi menutup sebuah grup secara
  // manual tidak akan langsung dibuka paksa lagi -- baru terjadi kalau
  // pengguna benar-benar berpindah halaman.
  useEffect(() => {
    const activeId = findModuleIdForPath(moduleTree, pathname)
    if (activeId) setOpenModuleId(activeId)
  }, [moduleTree, pathname])

  return (
    <aside className={`edp-sidebar d-flex flex-column p-3 ${collapsed ? 'is-collapsed' : ''}`}>
      <div className="mb-4">
        <Logo />
      </div>

      <nav className="flex-grow-1 overflow-y-auto overflow-x-hidden">
        <div className="mb-3">
          <div className="edp-nav-section-label px-2 mb-1">Utama</div>
          <ul className="nav nav-pills flex-column gap-1">
            <li>
              <NavLink
                to="/"
                end
                onClick={onNavigate}
                className={({ isActive }) =>
                  `edp-nav-link nav-link d-flex align-items-center gap-2 px-2 py-2 ${isActive ? 'active' : ''}`
                }
              >
                <i className="bi bi-speedometer2" />
                Dashboard
              </NavLink>
            </li>
          </ul>
        </div>

        {menuError && <div className="text-danger small px-2 mb-2">{menuError}</div>}
        {!moduleTree && !menuError && <div className="text-secondary small px-2 mb-2">Memuat menu...</div>}

        {moduleTree?.map((mod) => {
          const isOpen = openModuleId === mod.id
          const hasActivePage = moduleContainsPath(mod, pathname)
          return (
            <div className="mb-1" key={mod.id}>
              <button
                type="button"
                className={`edp-nav-section-toggle ${hasActivePage ? 'has-active' : ''}`}
                aria-expanded={isOpen}
                aria-controls={`edp-nav-group-${mod.id}`}
                onClick={() => setOpenModuleId(isOpen ? null : mod.id)}
              >
                <span className="edp-nav-section-label">{mod.name}</span>
                <i className={`bi bi-chevron-down edp-nav-chevron ${isOpen ? 'is-open' : ''}`} />
              </button>
              {isOpen && (
                <ul id={`edp-nav-group-${mod.id}`} className="nav nav-pills flex-column gap-1 mt-1">
                  {mod.menus.map((menu) => (
                    <MenuItem key={menu.id} menu={menu} onNavigate={onNavigate} />
                  ))}
                </ul>
              )}
            </div>
          )
        })}
      </nav>

      <div className="border-top border-secondary-subtle pt-3 mt-auto">
        <span className="edp-nav-section-label">Menu dari database &middot; role_menu_permissions</span>
      </div>
    </aside>
  )
}

export default Sidebar
