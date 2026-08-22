import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { category_code: '', name: '', description: '' }

function TicketCategoriesPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [categories, setCategories] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [isActive, setIsActive] = useState(true)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadCategories(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/ticketing/categories', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setCategories(data))
      .catch(() => setError('Gagal memuat data category. Pastikan ticketing-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadCategories(companyId, branchId)
  }, [companyId, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setIsActive(true)
    setFormError('')
    setEditing(true)
  }

  function openEdit(c) {
    setEditingId(c.id)
    setForm({
      category_code: c.category_code,
      name: c.name,
      description: c.description ?? '',
    })
    setIsActive(c.is_active)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/ticketing/categories/${editingId}`, {
          name: form.name,
          description: form.description,
          is_active: isActive,
        })
      } else {
        await apiClient.post('/api/ticketing/categories', {
          company_id: companyId,
          branch_id: branchId || null,
          category_code: form.category_code,
          name: form.name,
          description: form.description,
        })
      }
      setEditing(false)
      loadCategories(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data category')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'category_code', label: 'Kode', render: (c) => <code>{c.category_code}</code> },
    {
      key: 'name',
      label: 'Nama',
      render: (c) => (
        <div>
          <div>{c.name}</div>
          <div className="text-secondary small">{c.description}</div>
        </div>
      ),
    },
    {
      key: 'is_active',
      label: 'Status',
      render: (c) => <span className={`badge ${c.is_active ? 'text-bg-success' : 'text-bg-secondary'}`}>{c.is_active ? 'Aktif' : 'Nonaktif'}</span>,
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
          <h2 className="edp-page-title">Ticket Categories</h2>
          <div className="text-secondary small">Kategori tiket dukungan (Billing, Technical, Feature Request, dst).</div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Category
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={categories} loading={loading} searchPlaceholder="Cari kode atau nama category..." emptyMessage="Belum ada category." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Category' : 'Tambah Category'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="category-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="category-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Category</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.category_code}
                  onChange={(e) => setForm({ ...form, category_code: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Nama</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              <div className="col-12">
                <label className="form-label">Deskripsi</label>
                <input type="text" className="form-control" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </div>
              {editingId && (
                <div className="col-12 form-check">
                  <input
                    type="checkbox"
                    className="form-check-input"
                    id="category-is-active"
                    checked={isActive}
                    onChange={(e) => setIsActive(e.target.checked)}
                  />
                  <label className="form-check-label" htmlFor="category-is-active">Aktif</label>
                </div>
              )}
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default TicketCategoriesPage
