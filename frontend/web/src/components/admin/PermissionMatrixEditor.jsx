import { useState } from 'react'

export const PERMISSION_ACTIONS = [
  { key: 'can_view', label: 'Lihat' },
  { key: 'can_create', label: 'Tambah' },
  { key: 'can_update', label: 'Ubah' },
  { key: 'can_delete', label: 'Hapus' },
  { key: 'can_approve', label: 'Approve' },
  { key: 'can_export', label: 'Export' },
]

// Menampilkan seluruh menu (dikelompokkan per module, collapsible) dengan
// switch per aksi -- dipakai baik saat membuat role baru maupun mengedit
// permission role yang sudah ada. "Lihat" wajib aktif supaya menu muncul di
// sidebar; mengaktifkan aksi lain otomatis menyalakan "Lihat", mematikan
// "Lihat" mematikan seluruh aksi lain. Tiap module diringkas jadi satu baris
// (nama + jumlah menu aktif) yang bisa dibuka untuk atur per-menu.
function PermissionMatrixEditor({ modules, menus, onChange }) {
  const [expanded, setExpanded] = useState({})

  function toggle(menuId, actionKey) {
    onChange(
      menus.map((m) => {
        if (m.id !== menuId) return m
        const next = { ...m, [actionKey]: !m[actionKey] }
        if (actionKey !== 'can_view' && next[actionKey] && !next.can_view) {
          next.can_view = true
        }
        if (actionKey === 'can_view' && !next.can_view) {
          PERMISSION_ACTIONS.forEach((a) => (next[a.key] = false))
        }
        return next
      })
    )
  }

  function toggleModuleColumn(moduleId, actionKey, value) {
    onChange(
      menus.map((m) => {
        if (m.module_id !== moduleId) return m
        const next = { ...m, [actionKey]: value }
        if (actionKey === 'can_view' && !value) {
          PERMISSION_ACTIONS.forEach((a) => (next[a.key] = false))
        } else if (actionKey !== 'can_view' && value) {
          next.can_view = true
        }
        return next
      })
    )
  }

  return (
    <div className="d-flex flex-column gap-2">
      {modules.map((mod) => {
        const moduleMenus = menus.filter((m) => m.module_id === mod.id)
        if (moduleMenus.length === 0) return null
        const activeCount = moduleMenus.filter((m) => m.can_view).length
        const isOpen = Boolean(expanded[mod.id])

        return (
          <div className="card" key={mod.id}>
            <div className="edp-permission-module-header">
              <button
                type="button"
                className="edp-permission-module-toggle"
                onClick={() => setExpanded((prev) => ({ ...prev, [mod.id]: !prev[mod.id] }))}
                aria-expanded={isOpen}
              >
                <i className={`bi bi-chevron-${isOpen ? 'down' : 'right'}`} />
                <span className="fw-semibold">{mod.name}</span>
                <span className="edp-permission-module-count">
                  {activeCount} dari {moduleMenus.length} menu aktif
                </span>
              </button>
              <div className="d-flex gap-1 flex-wrap">
                {PERMISSION_ACTIONS.map((a) => (
                  <button
                    key={a.key}
                    type="button"
                    className="btn btn-sm btn-outline-secondary py-0 px-2"
                    style={{ fontSize: '0.7rem' }}
                    onClick={() => toggleModuleColumn(mod.id, a.key, true)}
                    title={`Aktifkan ${a.label} untuk semua menu ${mod.name}`}
                  >
                    Semua {a.label}
                  </button>
                ))}
              </div>
            </div>

            {isOpen && (
              <div className="table-responsive">
                <table className="table table-sm align-middle mb-0">
                  <thead>
                    <tr>
                      <th style={{ minWidth: 200 }}>Menu</th>
                      {PERMISSION_ACTIONS.map((a) => (
                        <th key={a.key} className="text-center" style={{ width: 80 }}>
                          {a.label}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {moduleMenus.map((m) => (
                      <tr key={m.id}>
                        <td>
                          {m.icon && <i className={`bi ${m.icon} me-2 text-secondary`} />}
                          {m.name}
                        </td>
                        {PERMISSION_ACTIONS.map((a) => (
                          <td key={a.key} className="text-center">
                            <div className="form-check form-switch d-inline-block m-0">
                              <input
                                type="checkbox"
                                role="switch"
                                className="form-check-input"
                                checked={Boolean(m[a.key])}
                                onChange={() => toggle(m.id, a.key)}
                              />
                            </div>
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

export default PermissionMatrixEditor
