import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import apiClient from '../../../services/apiClient.js'
import PermissionMatrixEditor from '../../../components/admin/PermissionMatrixEditor.jsx'

function RolePermissionMatrixPage() {
  const { roleId } = useParams()
  const [role, setRole] = useState(null)
  const [modules, setModules] = useState([])
  const [menus, setMenus] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [savedAt, setSavedAt] = useState(null)

  useEffect(() => {
    setLoading(true)
    Promise.all([
      apiClient.get('/api/rbac/roles'),
      apiClient.get('/api/rbac/modules'),
      apiClient.get(`/api/rbac/roles/${roleId}/permissions`),
    ])
      .then(([rolesRes, modulesRes, permissionsRes]) => {
        setRole(rolesRes.data.find((r) => r.id === roleId) ?? null)
        setModules(modulesRes.data)
        setMenus(permissionsRes.data)
      })
      .catch(() => setError('Gagal memuat data permission. Pastikan rbac-service aktif.'))
      .finally(() => setLoading(false))
  }, [roleId])

  async function handleSave() {
    setSaving(true)
    setError('')
    try {
      const payload = menus.map((m) => ({
        menu_id: m.id,
        can_view: m.can_view,
        can_create: m.can_create,
        can_update: m.can_update,
        can_delete: m.can_delete,
        can_approve: m.can_approve,
        can_export: m.can_export,
      }))
      const { data } = await apiClient.put(`/api/rbac/roles/${roleId}/permissions`, payload)
      setMenus(data)
      setSavedAt(new Date())
    } catch {
      setError('Gagal menyimpan permission.')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <div className="text-secondary">Memuat...</div>
  }

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <Link to="/admin/roles" className="text-decoration-none small">
            <i className="bi bi-arrow-left me-1" />
            Role Management
          </Link>
          <h2 className="edp-page-title mt-1">Permission &mdash; {role?.name ?? roleId}</h2>
          <div className="text-secondary small">
            Centang hak akses per menu. "Lihat" wajib aktif supaya menu muncul di sidebar.
          </div>
        </div>
        <div className="d-flex align-items-center gap-2">
          {savedAt && <span className="text-success small">Tersimpan {savedAt.toLocaleTimeString('id-ID')}</span>}
          <button type="button" className="btn btn-primary btn-sm" onClick={handleSave} disabled={saving}>
            {saving ? 'Menyimpan...' : 'Simpan Permission'}
          </button>
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <PermissionMatrixEditor modules={modules} menus={menus} onChange={setMenus} />
    </div>
  )
}

export default RolePermissionMatrixPage
