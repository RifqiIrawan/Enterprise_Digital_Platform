import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import apiClient from '../../../services/apiClient.js'
import PermissionMatrixEditor, { PERMISSION_ACTIONS } from '../../../components/admin/PermissionMatrixEditor.jsx'

const emptyActions = Object.fromEntries(PERMISSION_ACTIONS.map((a) => [a.key, false]))

function RoleCreatePage() {
  const navigate = useNavigate()
  const [modules, setModules] = useState([])
  const [menus, setMenus] = useState([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')

  const [form, setForm] = useState({ code: '', name: '', description: '' })
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  useEffect(() => {
    setLoading(true)
    Promise.all([apiClient.get('/api/rbac/modules'), apiClient.get('/api/rbac/menus')])
      .then(([modulesRes, menusRes]) => {
        setModules(modulesRes.data)
        setMenus(menusRes.data.map((m) => ({ ...m, ...emptyActions })))
      })
      .catch(() => setLoadError('Gagal memuat daftar menu. Pastikan rbac-service aktif.'))
      .finally(() => setLoading(false))
  }, [])

  const selectedCount = menus.filter((m) => m.can_view).length

  async function handleSubmit(e) {
    e.preventDefault()
    if (!form.code.trim() || !form.name.trim()) return
    setSaving(true)
    setFormError('')
    try {
      const { data: role } = await apiClient.post('/api/rbac/roles', form)

      const permissionPayload = menus
        .filter((m) => m.can_view)
        .map((m) => ({
          menu_id: m.id,
          can_view: m.can_view,
          can_create: m.can_create,
          can_update: m.can_update,
          can_delete: m.can_delete,
          can_approve: m.can_approve,
          can_export: m.can_export,
        }))
      if (permissionPayload.length > 0) {
        await apiClient.put(`/api/rbac/roles/${role.id}/permissions`, permissionPayload)
      }

      navigate('/admin/roles')
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat role')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <Link to="/admin/roles" className="text-decoration-none small">
            <i className="bi bi-arrow-left me-1" />
            Role Management
          </Link>
          <h2 className="edp-page-title mt-1">Tambah Role</h2>
          <div className="text-secondary small">
            Isi info role, lalu centang menu &amp; aksi yang boleh diakses role ini.
          </div>
        </div>
        <button type="submit" form="role-create-form" className="btn btn-primary btn-sm" disabled={saving}>
          {saving ? 'Menyimpan...' : `Simpan Role${selectedCount > 0 ? ` (${selectedCount} menu)` : ''}`}
        </button>
      </div>

      <form id="role-create-form" onSubmit={handleSubmit}>
        <div className="card mb-3">
          <div className="card-body">
            {formError && <div className="alert alert-danger py-2 small">{formError}</div>}
            <div className="row g-3">
              <div className="col-12 col-md-4">
                <label className="form-label">Code</label>
                <input
                  type="text"
                  className="form-control"
                  placeholder="mis. finance_viewer"
                  value={form.code}
                  onChange={(e) => setForm({ ...form, code: e.target.value })}
                  required
                />
              </div>
              <div className="col-12 col-md-4">
                <label className="form-label">Nama</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              <div className="col-12 col-md-4">
                <label className="form-label">Deskripsi</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                />
              </div>
            </div>
          </div>
        </div>

        {loadError && <div className="alert alert-danger py-2 small">{loadError}</div>}
        {loading && <div className="text-secondary">Memuat daftar menu...</div>}
        {!loading && !loadError && <PermissionMatrixEditor modules={modules} menus={menus} onChange={setMenus} />}
      </form>
    </div>
  )
}

export default RoleCreatePage
