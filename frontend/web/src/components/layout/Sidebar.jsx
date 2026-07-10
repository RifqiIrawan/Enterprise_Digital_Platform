import { NavLink } from 'react-router-dom'
import Logo from '../common/Logo.jsx'

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
  return (
    <aside className={`edp-sidebar d-flex flex-column p-3 ${collapsed ? 'is-collapsed' : ''}`}>
      <div className="mb-4">
        <Logo />
      </div>

      <nav className="flex-grow-1 overflow-auto">
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

        {moduleTree?.map((mod) => (
          <div className="mb-3" key={mod.id}>
            <div className="edp-nav-section-label px-2 mb-1">{mod.name}</div>
            <ul className="nav nav-pills flex-column gap-1">
              {mod.menus.map((menu) => (
                <MenuItem key={menu.id} menu={menu} onNavigate={onNavigate} />
              ))}
            </ul>
          </div>
        ))}
      </nav>

      <div className="border-top border-secondary-subtle pt-3 mt-auto">
        <span className="edp-nav-section-label">Menu dari database &middot; role_menu_permissions</span>
      </div>
    </aside>
  )
}

export default Sidebar
