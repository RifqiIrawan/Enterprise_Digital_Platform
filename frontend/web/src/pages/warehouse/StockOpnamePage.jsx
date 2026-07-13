import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyLine = { product_id: '', counted_quantity: 0 }
const emptyForm = { warehouse_id: '', opname_date: new Date().toISOString().slice(0, 10), notes: '', lines: [{ ...emptyLine }] }

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  POSTED: 'text-bg-success',
}

function StockOpnamePage() {
  const { companyId, branchId } = useCompany()
  const [warehouses, setWarehouses] = useState([])
  const [products, setProducts] = useState([])
  const [opnames, setOpnames] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const [viewing, setViewing] = useState(null)
  const [posting, setPosting] = useState(false)

  function loadOpnames(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/warehouse/stock-opnames', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setOpnames(data))
      .catch(() => setError('Gagal memuat data stock opname. Pastikan warehouse-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadOpnames(companyId, branchId)
    apiClient.get('/api/warehouse/warehouses', { params: { company_id: companyId } }).then(({ data }) => setWarehouses(data))
    apiClient.get('/api/warehouse/products', { params: { company_id: companyId } }).then(({ data }) => setProducts(data))
  }, [companyId, branchId])

  const warehouseName = (id) => warehouses.find((w) => w.id === id)?.name ?? id

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
      await apiClient.post('/api/warehouse/stock-opnames', {
        company_id: companyId,
        branch_id: branchId || null,
        warehouse_id: form.warehouse_id,
        opname_date: form.opname_date,
        notes: form.notes,
        lines: form.lines
          .filter((l) => l.product_id)
          .map((l) => ({ product_id: l.product_id, counted_quantity: Number(l.counted_quantity) || 0 })),
      })
      setCreating(false)
      loadOpnames(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat stock opname')
    } finally {
      setSaving(false)
    }
  }

  function openView(o) {
    apiClient.get(`/api/warehouse/stock-opnames/${o.id}`).then(({ data }) => setViewing(data))
  }

  async function handlePost() {
    setPosting(true)
    try {
      await apiClient.post(`/api/warehouse/stock-opnames/${viewing.id}/post`)
      setViewing(null)
      loadOpnames(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal memposting stock opname')
    } finally {
      setPosting(false)
    }
  }

  const columns = [
    { key: 'opname_number', label: 'No. Opname', render: (o) => <code>{o.opname_number}</code> },
    { key: 'warehouse_id', label: 'Gudang', render: (o) => warehouseName(o.warehouse_id), sortValue: (o) => warehouseName(o.warehouse_id) },
    {
      key: 'opname_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (o) => new Date(o.opname_date).toLocaleDateString('id-ID'),
    },
    {
      key: 'status',
      label: 'Status',
      render: (o) => <span className={`badge ${STATUS_BADGE[o.status] ?? 'text-bg-secondary'}`}>{o.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (o) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openView(o)}>
          {o.status === 'DRAFT' ? 'Lihat & Post' : 'Lihat'}
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Stock Opname</h2>
          <div className="text-secondary small">Catat hasil hitung fisik gudang; selisih baru diterapkan ke saldo saat di-post.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Buat Stock Opname
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={opnames} loading={loading} searchPlaceholder="Cari no. opname..." emptyMessage="Belum ada stock opname." />
      </div>

      {creating && (
        <Modal
          title="Buat Stock Opname"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="opname-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan sebagai Draft'}
              </button>
            </>
          }
        >
          <form id="opname-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Gudang</label>
                <select
                  className="form-select"
                  value={form.warehouse_id}
                  onChange={(e) => setForm({ ...form, warehouse_id: e.target.value })}
                  required
                >
                  <option value="">Pilih gudang...</option>
                  {warehouses.map((wh) => (
                    <option key={wh.id} value={wh.id}>{wh.code} - {wh.name}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.opname_date}
                  onChange={(e) => setForm({ ...form, opname_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-12">
                <label className="form-label">Catatan</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.notes}
                  onChange={(e) => setForm({ ...form, notes: e.target.value })}
                />
              </div>
            </div>

            <div>
              <div className="d-flex justify-content-between align-items-center mb-2">
                <label className="form-label mb-0">Hasil Hitung Fisik</label>
                <button type="button" className="btn btn-sm btn-outline-secondary" onClick={addLine}>
                  <i className="bi bi-plus-lg me-1" />
                  Baris
                </button>
              </div>
              <div className="table-responsive">
                <table className="table table-sm align-middle mb-0">
                  <thead>
                    <tr>
                      <th>Produk</th>
                      <th style={{ width: 120 }}>Qty Fisik</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {form.lines.map((l, i) => (
                      <tr key={i}>
                        <td>
                          <select
                            className="form-select form-select-sm"
                            value={l.product_id}
                            onChange={(e) => updateLine(i, { product_id: e.target.value })}
                          >
                            <option value="">Pilih produk...</option>
                            {products.map((p) => (
                              <option key={p.id} value={p.id}>{p.sku} - {p.name}</option>
                            ))}
                          </select>
                        </td>
                        <td>
                          <input
                            type="number"
                            className="form-control form-control-sm"
                            value={l.counted_quantity}
                            onChange={(e) => updateLine(i, { counted_quantity: e.target.value })}
                            min="0"
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

      {viewing && (
        <Modal
          title={`Stock Opname ${viewing.opname_number}`}
          onClose={() => setViewing(null)}
          footer={
            viewing.status === 'DRAFT' && (
              <>
                <button type="button" className="btn btn-outline-secondary" onClick={() => setViewing(null)}>
                  Tutup
                </button>
                <button type="button" className="btn btn-primary" disabled={posting} onClick={handlePost}>
                  {posting ? 'Memposting...' : 'Post & Sesuaikan Stok'}
                </button>
              </>
            )
          }
        >
          <div className="d-flex flex-column gap-2">
            <div className="text-secondary small">
              Gudang {warehouseName(viewing.warehouse_id)} &middot; {new Date(viewing.opname_date).toLocaleDateString('id-ID')}
            </div>
            <div className="table-responsive">
              <table className="table table-sm align-middle mb-0">
                <thead>
                  <tr>
                    <th>Produk</th>
                    <th className="text-end">Sistem</th>
                    <th className="text-end">Fisik</th>
                    <th className="text-end">Selisih</th>
                  </tr>
                </thead>
                <tbody>
                  {viewing.lines.map((l) => {
                    const diff = l.counted_quantity - l.system_quantity
                    return (
                      <tr key={l.id}>
                        <td>{l.product_name}</td>
                        <td className="text-end">{l.system_quantity}</td>
                        <td className="text-end">{l.counted_quantity}</td>
                        <td className={`text-end fw-semibold ${diff > 0 ? 'text-success' : diff < 0 ? 'text-danger' : ''}`}>
                          {diff > 0 ? `+${diff}` : diff}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

export default StockOpnamePage
