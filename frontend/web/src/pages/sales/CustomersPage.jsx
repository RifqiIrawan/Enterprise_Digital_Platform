import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'

const emptyForm = { customer_code: '', name: '', email: '', phone: '', address: '', tax_id: '', is_active: true }

function CustomersPage() {
  const [companyId, setCompanyId] = useState('')
  const [customers, setCustomers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadCustomers(cid) {
    setLoading(true)
    apiClient
      .get('/api/sales/customers', { params: { company_id: cid } })
      .then(({ data }) => setCustomers(data))
      .catch(() => setError('Gagal memuat data customer. Pastikan sales-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) loadCustomers(cid)
        else setLoading(false)
      })
      .catch(() => {
        setError('Gagal memuat data company.')
        setLoading(false)
      })
  }, [])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(c) {
    setEditingId(c.id)
    setForm({
      customer_code: c.customer_code,
      name: c.name,
      email: c.email ?? '',
      phone: c.phone ?? '',
      address: c.address ?? '',
      tax_id: c.tax_id ?? '',
      is_active: c.is_active,
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
        await apiClient.put(`/api/sales/customers/${editingId}`, {
          name: form.name,
          email: form.email,
          phone: form.phone,
          address: form.address,
          tax_id: form.tax_id,
          is_active: form.is_active,
        })
      } else {
        await apiClient.post('/api/sales/customers', {
          company_id: companyId,
          customer_code: form.customer_code,
          name: form.name,
          email: form.email,
          phone: form.phone,
          address: form.address,
          tax_id: form.tax_id,
        })
      }
      setEditing(false)
      loadCustomers(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data customer')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'customer_code', label: 'Kode', render: (c) => <code>{c.customer_code}</code> },
    {
      key: 'name',
      label: 'Nama',
      render: (c) => (
        <div>
          <div>{c.name}</div>
          <div className="text-secondary small">{c.email}</div>
        </div>
      ),
    },
    { key: 'phone', label: 'Telepon', cellClassName: 'text-secondary small' },
    {
      key: 'is_active',
      label: 'Status',
      render: (c) => <span className={`badge ${c.is_active ? 'text-bg-success' : 'text-bg-secondary'}`}>{c.is_active ? 'Aktif' : 'Nonaktif'}</span>,
      sortValue: (c) => (c.is_active ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (c) => (
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
          <h2 className="edp-page-title">Customers</h2>
          <div className="text-secondary small">Master customer untuk quotation &amp; sales order.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Customer
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={customers}
          loading={loading}
          searchPlaceholder="Cari kode, nama, atau email..."
          emptyMessage="Belum ada customer. Tambahkan data customer terlebih dahulu."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Customer' : 'Tambah Customer'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="customer-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="customer-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Customer</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.customer_code}
                  onChange={(e) => setForm({ ...form, customer_code: e.target.value })}
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
                      id="customer-is-active"
                      checked={form.is_active}
                      onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
                    />
                    <label className="form-check-label" htmlFor="customer-is-active">Aktif</label>
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

export default CustomersPage
