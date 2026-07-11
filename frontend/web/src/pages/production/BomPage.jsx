import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyLine = { component_product_id: '', quantity_per_unit: 1 }
const emptyForm = { bom_code: '', name: '', product_id: '', lines: [{ ...emptyLine }] }

function BomPage() {
  const { companyId } = useCompany()
  const [products, setProducts] = useState([])
  const [boms, setBoms] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const [editing, setEditing] = useState(null)
  const [editForm, setEditForm] = useState({ name: '', is_active: true })
  const [editError, setEditError] = useState('')
  const [editSaving, setEditSaving] = useState(false)

  function loadBoms(cid) {
    setLoading(true)
    apiClient
      .get('/api/production/boms', { params: { company_id: cid } })
      .then(({ data }) => setBoms(data))
      .catch(() => setError('Gagal memuat data BOM. Pastikan production-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadBoms(companyId)
    apiClient.get('/api/warehouse/products', { params: { company_id: companyId } }).then(({ data }) => setProducts(data))
  }, [companyId])

  const productName = (id) => {
    const p = products.find((p) => p.id === id)
    return p ? `${p.sku} - ${p.name}` : id
  }

  function updateLine(index, patch) {
    setForm((f) => ({ ...f, lines: f.lines.map((l, i) => (i === index ? { ...l, ...patch } : l)) }))
  }

  function addLine() {
    setForm((f) => ({ ...f, lines: [...f.lines, { ...emptyLine }] }))
  }

  function removeLine(index) {
    setForm((f) => ({ ...f, lines: f.lines.filter((_, i) => i !== index) }))
  }

  function openCreate() {
    setForm({ ...emptyForm, lines: [{ ...emptyLine }] })
    setFormError('')
    setCreating(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/production/boms', {
        company_id: companyId,
        bom_code: form.bom_code,
        name: form.name,
        product_id: form.product_id,
        lines: form.lines
          .filter((l) => l.component_product_id)
          .map((l) => ({ component_product_id: l.component_product_id, quantity_per_unit: Number(l.quantity_per_unit) || 0 })),
      })
      setCreating(false)
      loadBoms(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat BOM')
    } finally {
      setSaving(false)
    }
  }

  function openEdit(b) {
    setEditing(b)
    setEditForm({ name: b.name, is_active: b.is_active })
    setEditError('')
  }

  async function handleEdit(e) {
    e.preventDefault()
    setEditSaving(true)
    setEditError('')
    try {
      await apiClient.put(`/api/production/boms/${editing.id}`, editForm)
      setEditing(null)
      loadBoms(companyId)
    } catch (err) {
      setEditError(err.response?.data?.error ?? 'Gagal menyimpan BOM')
    } finally {
      setEditSaving(false)
    }
  }

  const columns = [
    { key: 'bom_code', label: 'Kode BOM', render: (b) => <code>{b.bom_code}</code> },
    { key: 'name', label: 'Nama' },
    { key: 'product_id', label: 'Produk Jadi', render: (b) => productName(b.product_id), sortValue: (b) => productName(b.product_id) },
    {
      key: 'is_active',
      label: 'Status',
      render: (b) => <span className={`badge ${b.is_active ? 'text-bg-success' : 'text-bg-secondary'}`}>{b.is_active ? 'Aktif' : 'Nonaktif'}</span>,
      sortValue: (b) => (b.is_active ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (b) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(b)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Bill of Material</h2>
          <div className="text-secondary small">Resep komponen untuk tiap produk jadi yang diproduksi.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Buat BOM
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={boms} loading={loading} searchPlaceholder="Cari kode atau nama BOM..." emptyMessage="Belum ada BOM." />
      </div>

      {creating && (
        <Modal
          title="Buat Bill of Material"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="bom-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="bom-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode BOM</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.bom_code}
                  onChange={(e) => setForm({ ...form, bom_code: e.target.value })}
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
                <label className="form-label">Produk Jadi</label>
                <select
                  className="form-select"
                  value={form.product_id}
                  onChange={(e) => setForm({ ...form, product_id: e.target.value })}
                  required
                >
                  <option value="">Pilih produk...</option>
                  {products.map((p) => (
                    <option key={p.id} value={p.id}>{p.sku} - {p.name}</option>
                  ))}
                </select>
              </div>
            </div>

            <div>
              <div className="d-flex justify-content-between align-items-center mb-2">
                <label className="form-label mb-0">Komponen / Bahan Baku</label>
                <button type="button" className="btn btn-sm btn-outline-secondary" onClick={addLine}>
                  <i className="bi bi-plus-lg me-1" />
                  Baris
                </button>
              </div>
              <div className="table-responsive">
                <table className="table table-sm align-middle mb-0">
                  <thead>
                    <tr>
                      <th>Komponen</th>
                      <th style={{ width: 140 }}>Qty per Unit</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {form.lines.map((l, i) => (
                      <tr key={i}>
                        <td>
                          <select
                            className="form-select form-select-sm"
                            value={l.component_product_id}
                            onChange={(e) => updateLine(i, { component_product_id: e.target.value })}
                          >
                            <option value="">Pilih komponen...</option>
                            {products.map((p) => (
                              <option key={p.id} value={p.id}>{p.sku} - {p.name}</option>
                            ))}
                          </select>
                        </td>
                        <td>
                          <input
                            type="number"
                            className="form-control form-control-sm"
                            value={l.quantity_per_unit}
                            onChange={(e) => updateLine(i, { quantity_per_unit: e.target.value })}
                            min="0"
                            step="0.01"
                          />
                        </td>
                        <td>
                          {form.lines.length > 1 && (
                            <button type="button" className="btn btn-sm btn-outline-danger" onClick={() => removeLine(i)}>
                              <i className="bi bi-x" />
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          </form>
        </Modal>
      )}

      {editing && (
        <Modal
          title={`Edit BOM ${editing.bom_code}`}
          onClose={() => setEditing(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(null)}>
                Batal
              </button>
              <button type="submit" form="bom-edit-form" className="btn btn-primary" disabled={editSaving}>
                {editSaving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="bom-edit-form" onSubmit={handleEdit} className="d-flex flex-column gap-3">
            {editError && <div className="alert alert-danger py-2 small mb-0">{editError}</div>}
            <div>
              <label className="form-label">Nama</label>
              <input
                type="text"
                className="form-control"
                value={editForm.name}
                onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
                required
              />
            </div>
            <div className="form-check">
              <input
                type="checkbox"
                className="form-check-input"
                id="bom-is-active"
                checked={editForm.is_active}
                onChange={(e) => setEditForm({ ...editForm, is_active: e.target.checked })}
              />
              <label className="form-check-label" htmlFor="bom-is-active">Aktif</label>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default BomPage
