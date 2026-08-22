import { useEffect, useMemo, useState } from 'react'
import apiClient from '../../../services/apiClient.js'
import Modal from '../../../components/common/Modal.jsx'

const ACTIONS = [
  { key: 'can_view', label: 'Lihat' },
  { key: 'can_create', label: 'Buat' },
  { key: 'can_update', label: 'Ubah' },
  { key: 'can_delete', label: 'Hapus' },
  { key: 'can_approve', label: 'Setujui' },
  { key: 'can_export', label: 'Ekspor' },
]

function sameActions(a, b) {
  return ACTIONS.every(({ key }) => Boolean(a[key]) === Boolean(b[key]))
}

// UserAccessModal menampilkan hak EFEKTIF seorang user per menu (gabungan
// seluruh role-nya) dan memungkinkan menimpanya khusus untuk user ini lewat
// user_menu_permission_overrides. Baris yang berbeda dari hak bawaan role
// ditandai supaya jelas mana yang menyimpang -- dan bisa dikembalikan.
function UserAccessModal({ user, companyId, onClose }) {
  const [rows, setRows] = useState([])
  const [overrideIdByMenu, setOverrideIdByMenu] = useState({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  function load() {
    setLoading(true)
    Promise.all([
      apiClient.get('/api/rbac/user-permissions', { params: { user_id: user.id, company_id: companyId } }),
      apiClient.get('/api/rbac/user-overrides', { params: { user_id: user.id, company_id: companyId } }),
    ])
      .then(([permsRes, overridesRes]) => {
        setRows(permsRes.data)
        setOverrideIdByMenu(Object.fromEntries(overridesRes.data.map((o) => [o.menu_id, o.id])))
        setError('')
      })
      .catch(() => setError('Gagal memuat hak akses user.'))
      .finally(() => setLoading(false))
  }

  useEffect(load, [user.id, companyId])

  const modules = useMemo(() => {
    const grouped = []
    rows.forEach((row) => {
      const last = grouped[grouped.length - 1]
      if (last && last.id === row.module_id) last.rows.push(row)
      else grouped.push({ id: row.module_id, name: row.module_name, rows: [row] })
    })
    return grouped
  }, [rows])

  const changedCount = rows.filter((row) => !sameActions(row, row.role_actions)).length

  function toggle(menuId, actionKey) {
    setRows((prev) =>
      prev.map((row) => {
        if (row.menu_id !== menuId) return row
        const next = { ...row, [actionKey]: !row[actionKey] }
        // Hak turunan tanpa hak lihat ditolak backend, jadi mematikan "Lihat"
        // ikut mematikan sisanya di sini -- bukan dibiarkan sampai gagal simpan.
        if (actionKey === 'can_view' && !next.can_view) {
          ACTIONS.forEach(({ key }) => {
            next[key] = false
          })
        }
        if (actionKey !== 'can_view' && next[actionKey]) next.can_view = true
        return next
      })
    )
  }

  function resetRow(menuId) {
    setRows((prev) =>
      prev.map((row) => (row.menu_id === menuId ? { ...row, ...row.role_actions } : row))
    )
  }

  async function handleSave() {
    setSaving(true)
    setError('')
    try {
      for (const row of rows) {
        const differs = !sameActions(row, row.role_actions)
        const overrideId = overrideIdByMenu[row.menu_id]
        if (differs) {
          await apiClient.put('/api/rbac/user-overrides', {
            user_id: user.id,
            company_id: companyId,
            menu_id: row.menu_id,
            ...Object.fromEntries(ACTIONS.map(({ key }) => [key, Boolean(row[key])])),
          })
        } else if (overrideId) {
          // Kembali sama dengan role = override-nya dihapus, bukan disimpan
          // sebagai salinan hak role yang akan basi kalau role diubah nanti.
          await apiClient.delete(`/api/rbac/user-overrides/${overrideId}`)
        }
      }
      load()
    } catch (err) {
      setError(err.response?.data?.error ?? 'Gagal menyimpan hak akses.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title={`Hak Akses — ${user.full_name}`}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn-outline-secondary" onClick={onClose}>
            Tutup
          </button>
          <button type="button" className="btn btn-primary" onClick={handleSave} disabled={saving || loading}>
            {saving ? 'Menyimpan...' : 'Simpan Perubahan'}
          </button>
        </>
      }
    >
      <div className="d-flex flex-column gap-3">
        {error && <div className="alert alert-danger py-2 small mb-0">{error}</div>}

        {user.is_super_admin && (
          <div className="alert alert-warning py-2 small mb-0">
            User ini Super Admin &mdash; dia melihat seluruh menu tanpa melewati role maupun override,
            jadi perubahan di sini tidak berpengaruh pada sidebar-nya.
          </div>
        )}

        <div className="small text-secondary">
          Centang untuk menimpa hak bawaan role khusus user ini.{' '}
          <span className="badge text-bg-info-subtle text-info-emphasis">override</span> menandai baris
          yang berbeda dari role. Menu tanpa hak lihat tidak muncul di sidebar user tersebut.
          {changedCount > 0 && <> Saat ini <strong>{changedCount}</strong> menu menyimpang dari role.</>}
        </div>

        {loading && <div className="text-secondary small">Memuat...</div>}

        {!loading &&
          modules.map((mod) => (
            <div key={mod.id}>
              <div className="fw-semibold small mb-1">{mod.name}</div>
              <div className="table-responsive">
                <table className="table table-sm align-middle mb-0">
                  <thead>
                    <tr>
                      <th style={{ minWidth: 160 }}>Menu</th>
                      {ACTIONS.map(({ key, label }) => (
                        <th key={key} className="text-center small">
                          {label}
                        </th>
                      ))}
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {mod.rows.map((row) => {
                      const differs = !sameActions(row, row.role_actions)
                      return (
                        <tr key={row.menu_id}>
                          <td>
                            {row.menu_name}
                            {differs && (
                              <span className="badge text-bg-info-subtle text-info-emphasis ms-2">override</span>
                            )}
                          </td>
                          {ACTIONS.map(({ key }) => (
                            <td key={key} className="text-center">
                              <input
                                type="checkbox"
                                className="form-check-input"
                                checked={Boolean(row[key])}
                                onChange={() => toggle(row.menu_id, key)}
                                aria-label={`${row.menu_name} ${key}`}
                              />
                            </td>
                          ))}
                          <td className="text-end">
                            {differs && (
                              <button
                                type="button"
                                className="btn btn-sm btn-link p-0 text-decoration-none"
                                onClick={() => resetRow(row.menu_id)}
                              >
                                Ikuti role
                              </button>
                            )}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
      </div>
    </Modal>
  )
}

export default UserAccessModal
