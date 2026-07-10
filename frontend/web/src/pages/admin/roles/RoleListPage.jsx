import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import apiClient from '../../../services/apiClient.js'
import Modal from '../../../components/common/Modal.jsx'
import DataTable from '../../../components/common/DataTable.jsx'

const emptyForm = { code: '', name: '', description: '' }

function roleColumns(openEdit, handleDelete) {
  return [
    { key: 'code', label: 'Code', render: (role) => <code>{role.code}</code> },
    { key: 'name', label: 'Nama' },
    { key: 'description', label: 'Deskripsi', maxWidth: 320, cellClassName: 'text-secondary small' },
    {
      key: 'is_system',
      label: 'Tipe',
      render: (role) =>
        role.is_system ? (
          <span className="badge text-bg-secondary">System</span>
        ) : (
          <span className="badge text-bg-light border">Custom</span>
        ),
      sortValue: (role) => (role.is_system ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (role) => (
        <>
          <Link to={`/admin/roles/${role.id}/permissions`} className="btn btn-sm btn-outline-primary me-1">
            <i className="bi bi-shield-check me-1" />
            Permission
          </Link>
          <button type="button" className="btn btn-sm btn-outline-secondary me-1" onClick={() => openEdit(role)}>
            <i className="bi bi-pencil" />
          </button>
          <button
            type="button"
            className="btn btn-sm btn-outline-danger"
            disabled={role.is_system}
            title={role.is_system ? 'Role sistem tidak boleh dihapus' : 'Hapus role'}
            onClick={() => handleDelete(role)}
          >
            <i className="bi bi-trash" />
          </button>
        </>
      ),
    },
  ]
}

function RoleListPage() {
  const navigate = useNavigate()
  const [roles, setRoles] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(null) // null = closed, role = edit
  const [form, setForm] = useState(emptyForm)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  function load() {
    setLoading(true)
    apiClient
      .get('/api/rbac/roles')
      .then(({ data }) => setRoles(data))
      .catch(() => setError('Gagal memuat daftar role. Pastikan rbac-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  function openEdit(role) {
    setForm({ code: role.code, name: role.name, description: role.description ?? '' })
    setFormError('')
    setEditing(role)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.put(`/api/rbac/roles/${editing.id}`, { name: form.name, description: form.description })
      setEditing(null)
      load()
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan role')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(role) {
    if (!window.confirm(`Hapus role "${role.name}"?`)) return
    try {
      await apiClient.delete(`/api/rbac/roles/${role.id}`)
      load()
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal menghapus role')
    }
  }

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Role Management</h2>
          <div className="text-secondary small">Master role &amp; hak akses menu per role.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" onClick={() => navigate('/admin/roles/new')}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Role
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={roleColumns(openEdit, handleDelete)}
          data={roles}
          loading={loading}
          searchPlaceholder="Cari code atau nama role..."
          emptyMessage="Belum ada role."
        />
      </div>

      {editing !== null && (
        <Modal
          title="Edit Role"
          onClose={() => setEditing(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(null)}>
                Batal
              </button>
              <button type="submit" form="role-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="role-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div>
              <label className="form-label">Code</label>
              <input type="text" className="form-control" value={form.code} disabled />
            </div>
            <div>
              <label className="form-label">Nama</label>
              <input
                type="text"
                className="form-control"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                required
              />
            </div>
            <div>
              <label className="form-label">Deskripsi</label>
              <textarea
                className="form-control"
                rows={2}
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
              />
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default RoleListPage
