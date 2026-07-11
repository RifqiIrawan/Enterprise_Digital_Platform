import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { supplier_code: '', name: '', email: '', phone: '', address: '', tax_id: '', is_active: true }

function SuppliersPage() {
  const { companyId } = useCompany()
  const [suppliers, setSuppliers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadSuppliers(cid) {
    setLoading(true)
    apiClient
      .get('/api/purchasing/suppliers', { params: { company_id: cid } })
      .then(({ data }) => setSuppliers(data))
      .catch(() => setError('Gagal memuat data supplier. Pastikan purchasing-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadSuppliers(companyId)
  }, [companyId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(s) {
    setEditingId(s.id)
    setForm({
      supplier_code: s.supplier_code,
      name: s.name,
      email: s.email ?? '',
      phone: s.phone ?? '',
      address: s.address ?? '',
      tax_id: s.tax_id ?? '',
      is_active: s.is_active,
    })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/purchasing/suppliers/${editingId}`, {
          name: form.name,
          email: form.email,
          phone: form.phone,
          address: form.address,
          tax_id: form.tax_id,
          is_active: form.is_active,
        })
      } else {
        await apiClient.post('/api/purchasing/suppliers', {
          company_id: companyId,
          supplier_code: form.supplier_code,
          name: form.name,
          email: form.email,
          phone: form.phone,
          address: form.address,
          tax_id: form.tax_id,
        })
      }
      setEditing(false)
      loadSuppliers(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data supplier')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'supplier_code', label: 'Kode', render: (s) => <code>{s.supplier_code}</code> },
    {
      key: 'name',
      label: 'Nama',
      render: (s) => (
        <div>
          <div>{s.name}</div>
          <div className="text-secondary small">{s.email}</div>
        </div>
      ),
    },
    { key: 'phone', label: 'Telepon', cellClassName: 'text-secondary small' },
    {
      key: 'is_active',
      label: 'Status',
      render: (s) => <span className={`badge ${s.is_active ? 'text-bg-success' : 'text-bg-secondary'}`}>{s.is_active ? 'Aktif' : 'Nonaktif'}</span>,
      sortValue: (s) => (s.is_active ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (s) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(s)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Suppliers</h2>
          <div className="text-secondary small">Master supplier untuk purchase requisition &amp; purchase order.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Supplier
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={suppliers}
          loading={loading}
          searchPlaceholder="Cari kode, nama, atau email..."
          emptyMessage="Belum ada supplier. Tambahkan data supplier terlebih dahulu."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Supplier' : 'Tambah Supplier'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="supplier-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="supplier-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Supplier</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.supplier_code}
                  onChange={(e) => setForm({ ...form, supplier_code: e.target.value })}
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
              <div className="col-6">
                <label className="form-label">Email</label>
                <input
                  type="email"
                  className="form-control"
                  value={form.email}
                  onChange={(e) => setForm({ ...form, email: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Telepon</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.phone}
                  onChange={(e) => setForm({ ...form, phone: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">NPWP</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.tax_id}
                  onChange={(e) => setForm({ ...form, tax_id: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Alamat</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.address}
                  onChange={(e) => setForm({ ...form, address: e.target.value })}
                />
              </div>
              {editingId && (
                <div className="col-6 d-flex align-items-end">
                  <div className="form-check">
                    <input
                      type="checkbox"
                      className="form-check-input"
                      id="supplier-is-active"
                      checked={form.is_active}
                      onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
                    />
                    <label className="form-check-label" htmlFor="supplier-is-active">Aktif</label>
                  </div>
                </div>
              )}
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default SuppliersPage
