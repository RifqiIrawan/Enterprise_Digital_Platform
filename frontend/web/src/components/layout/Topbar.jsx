import { useNavigate } from 'react-router-dom'
import { clearSession, getCurrentUser } from '../../utils/auth.js'

function initials(fullName) {
  return fullName
    .split(' ')
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toUpperCase())
    .join('')
}

function Topbar({ title, onToggleSidebar }) {
  const navigate = useNavigate()
  const user = getCurrentUser()

  function handleLogout() {
    clearSession()
    navigate('/login')
  }

  return (
    <header className="edp-topbar d-flex align-items-center justify-content-between px-3 px-md-4">
      <div className="d-flex align-items-center gap-3">
        <button
          type="button"
          className="btn btn-sm btn-outline-secondary d-flex align-items-center"
          onClick={onToggleSidebar}
          aria-label="Tampilkan/sembunyikan menu"
        >
          <i className="bi bi-list fs-5" />
        </button>
        <h1 className="h5 mb-0">{title}</h1>
      </div>

      <div className="dropdown">
        <button
          type="button"
          className="btn btn-link text-decoration-none d-flex align-items-center gap-2 p-0"
          data-bs-toggle="dropdown"
          aria-expanded="false"
        >
          <span className="edp-avatar">{initials(user?.full_name ?? '?')}</span>
          <span className="d-none d-sm-inline text-body">{user?.full_name}</span>
          <i className="bi bi-chevron-down small text-secondary" />
        </button>
        <ul className="dropdown-menu dropdown-menu-end">
          <li>
            <span className="dropdown-item-text text-secondary small">{user?.email}</span>
          </li>
          <li>
            <hr className="dropdown-divider" />
          </li>
          <li>
            <button type="button" className="dropdown-item" onClick={handleLogout}>
              <i className="bi bi-box-arrow-right me-2" />
              Keluar
            </button>
          </li>
        </ul>
      </div>
    </header>
  )
}

export default Topbar
