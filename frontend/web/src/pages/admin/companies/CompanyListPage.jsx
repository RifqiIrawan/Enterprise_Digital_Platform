import { useState } from 'react'
import apiClient from '../../../services/apiClient.js'
import Modal from '../../../components/common/Modal.jsx'
import DataTable from '../../../components/common/DataTable.jsx'
import { useCompany } from '../../../store/CompanyContext.jsx'
import { usePagePermission } from '../../../store/PermissionContext.jsx'

const emptyForm = { code: '', name: '', status: 'active' }

const STATUS_BADGE = {
  active: 'text-bg-success',
  inactive: 'text-bg-secondary',
}

// Halaman ini membaca daftarnya langsung dari CompanyContext, bukan fetch
// sendiri: context-lah yang jadi sumber kebenaran untuk switcher di Topbar,
// jadi menampilkan hasil fetch terpisah di sini justru bisa berbeda dari yang
// dipakai halaman lain. Setelah menyimpan, context-nya yang di-reload.
function CompanyListPage() {
  const { companies, companyId, loading, error, reloadCompanies } = useCompany()
  const { can } = usePagePermission()

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(c) {
    setEditingId(c.id)
    setForm({ code: c.code, name: c.name, status: c.status })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/company/companies/${editingId}`, { name: form.name, status: form.status })
      } else {
        await apiClient.post('/api/company/companies', { code: form.code, name: form.name })
      }
      await reloadCompanies()
      setEditing(false)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data company')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'code', label: 'Kode', render: (c) => <code>{c.code}</code> },
    {
      key: 'name',
      label: 'Nama Company',
      render: (c) => (
        <div>
          <div>{c.name}</div>
          {c.id === companyId && <div className="text-secondary small">Sedang aktif di switcher</div>}
        </div>
      ),
    },
    {
      key: 'status',
      label: 'Status',
      render: (c) => <span className={`badge ${STATUS_BADGE[c.status] ?? 'text-bg-secondary'}`}>{c.status}</span>,
    },
    {
      key: 'created_at',
      label: 'Dibuat',
      render: (c) => new Date(c.created_at).toLocaleDateString('id-ID'),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (c) => can('update') && (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(c)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Company Management</h2>
          <div className="text-secondary small">
            Company adalah akar seluruh data di platform ini. Kodenya tidak bisa diubah setelah dibuat.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Company
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={companies}
          loading={loading}
          searchPlaceholder="Cari kode atau nama company..."
          emptyMessage="Belum ada company."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Company' : 'Tambah Company'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="company-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="company-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-4">
                <label className="form-label">Kode</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.code}
                  onChange={(e) => setForm({ ...form, code: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-8">
                <label className="form-label">Nama Company</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              {editingId && (
                <div className="col-4">
                  <label className="form-label">Status</label>
                  <select
                    className="form-select"
                    value={form.status}
                    onChange={(e) => setForm({ ...form, status: e.target.value })}
                  >
                    <option value="active">active</option>
                    <option value="inactive">inactive</option>
                  </select>
                </div>
              )}
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default CompanyListPage
