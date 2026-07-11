import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'

const emptyForm = { sku: '', name: '', unit: 'pcs', category: '', cost_price: '', is_active: true }

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

function ProductsPage() {
  const [companyId, setCompanyId] = useState('')
  const [products, setProducts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadProducts(cid) {
    setLoading(true)
    apiClient
      .get('/api/warehouse/products', { params: { company_id: cid } })
      .then(({ data }) => setProducts(data))
      .catch(() => setError('Gagal memuat data produk. Pastikan warehouse-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) loadProducts(cid)
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

  function openEdit(p) {
    setEditingId(p.id)
    setForm({
      sku: p.sku,
      name: p.name,
      unit: p.unit,
      category: p.category ?? '',
      cost_price: p.cost_price ?? '',
      is_active: p.is_active,
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
        await apiClient.put(`/api/warehouse/products/${editingId}`, {
          name: form.name,
          unit: form.unit,
          category: form.category,
          cost_price: Number(form.cost_price) || 0,
          is_active: form.is_active,
        })
      } else {
        await apiClient.post('/api/warehouse/products', {
          company_id: companyId,
          sku: form.sku,
          name: form.name,
          unit: form.unit,
          category: form.category,
          cost_price: Number(form.cost_price) || 0,
        })
      }
      setEditing(false)
      loadProducts(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data produk')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'sku', label: 'SKU', render: (p) => <code>{p.sku}</code> },
    {
      key: 'name',
      label: 'Nama',
      render: (p) => (
        <div>
          <div>{p.name}</div>
          <div className="text-secondary small">{p.category}</div>
        </div>
      ),
    },
    { key: 'unit', label: 'Satuan', cellClassName: 'text-secondary small' },
    {
      key: 'cost_price',
      label: 'Harga Pokok',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (p) => formatMoney(p.cost_price),
    },
    {
      key: 'is_active',
      label: 'Status',
      render: (p) => <span className={`badge ${p.is_active ? 'text-bg-success' : 'text-bg-secondary'}`}>{p.is_active ? 'Aktif' : 'Nonaktif'}</span>,
      sortValue: (p) => (p.is_active ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (p) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(p)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Master Barang</h2>
          <div className="text-secondary small">Daftar produk yang dilacak stoknya di seluruh gudang.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Produk
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={products}
          loading={loading}
          searchPlaceholder="Cari SKU atau nama produk..."
          emptyMessage="Belum ada produk. Tambahkan data produk terlebih dahulu."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Produk' : 'Tambah Produk'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="product-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="product-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">SKU</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.sku}
                  onChange={(e) => setForm({ ...form, sku: e.target.value })}
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
                <label className="form-label">Satuan</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.unit}
                  onChange={(e) => setForm({ ...form, unit: e.target.value })}
                  placeholder="pcs, kg, box, ..."
                />
              </div>
              <div className="col-6">
                <label className="form-label">Kategori</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.category}
                  onChange={(e) => setForm({ ...form, category: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Harga Pokok</label>
                <input
                  type="number"
                  className="form-control"
                  value={form.cost_price}
                  onChange={(e) => setForm({ ...form, cost_price: e.target.value })}
                  min="0"
                />
              </div>
              {editingId && (
                <div className="col-6 d-flex align-items-end">
                  <div className="form-check">
                    <input
                      type="checkbox"
                      className="form-check-input"
                      id="product-is-active"
                      checked={form.is_active}
                      onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
                    />
                    <label className="form-check-label" htmlFor="product-is-active">Aktif</label>
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

export default ProductsPage
