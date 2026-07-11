import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'

const emptyLine = { product_id: '', quantity: 1 }
const emptyForm = { from_warehouse_id: '', to_warehouse_id: '', transfer_date: new Date().toISOString().slice(0, 10), notes: '', lines: [{ ...emptyLine }] }

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  CONFIRMED: 'text-bg-success',
  CANCELLED: 'text-bg-danger',
}

function StockTransfersPage() {
  const [companyId, setCompanyId] = useState('')
  const [warehouses, setWarehouses] = useState([])
  const [products, setProducts] = useState([])
  const [transfers, setTransfers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState(null)

  function loadTransfers(cid) {
    setLoading(true)
    apiClient
      .get('/api/warehouse/stock-transfers', { params: { company_id: cid } })
      .then(({ data }) => setTransfers(data))
      .catch(() => setError('Gagal memuat data mutasi antar gudang. Pastikan warehouse-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) {
          loadTransfers(cid)
          apiClient.get('/api/warehouse/warehouses', { params: { company_id: cid } }).then(({ data }) => setWarehouses(data))
          apiClient.get('/api/warehouse/products', { params: { company_id: cid } }).then(({ data }) => setProducts(data))
        } else {
          setLoading(false)
        }
      })
      .catch(() => {
        setError('Gagal memuat data company.')
        setLoading(false)
      })
  }, [])

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
      await apiClient.post('/api/warehouse/stock-transfers', {
        company_id: companyId,
        from_warehouse_id: form.from_warehouse_id,
        to_warehouse_id: form.to_warehouse_id,
        transfer_date: form.transfer_date,
        notes: form.notes,
        lines: form.lines
          .filter((l) => l.product_id)
          .map((l) => ({ product_id: l.product_id, quantity: Number(l.quantity) || 0 })),
      })
      setCreating(false)
      loadTransfers(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat mutasi antar gudang')
    } finally {
      setSaving(false)
    }
  }

  async function handleConfirm(id) {
    setActingId(id)
    try {
      await apiClient.post(`/api/warehouse/stock-transfers/${id}/confirm`)
      loadTransfers(companyId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal mengonfirmasi mutasi antar gudang')
    } finally {
      setActingId(null)
    }
  }

  const columns = [
    { key: 'transfer_number', label: 'No. Transfer', render: (t) => <code>{t.transfer_number}</code> },
    { key: 'from_warehouse_id', label: 'Dari', render: (t) => warehouseName(t.from_warehouse_id), sortValue: (t) => warehouseName(t.from_warehouse_id) },
    { key: 'to_warehouse_id', label: 'Ke', render: (t) => warehouseName(t.to_warehouse_id), sortValue: (t) => warehouseName(t.to_warehouse_id) },
    {
      key: 'transfer_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (t) => new Date(t.transfer_date).toLocaleDateString('id-ID'),
    },
    {
      key: 'status',
      label: 'Status',
      render: (t) => <span className={`badge ${STATUS_BADGE[t.status] ?? 'text-bg-secondary'}`}>{t.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (t) =>
        t.status === 'DRAFT' && (
          <button type="button" className="btn btn-sm btn-outline-success" disabled={actingId === t.id} onClick={() => handleConfirm(t.id)}>
            Konfirmasi
          </button>
        ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Mutasi Antar Gudang</h2>
          <div className="text-secondary small">Pindahkan stok antar gudang/branch. Stok baru berpindah setelah dikonfirmasi.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Buat Mutasi
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={transfers} loading={loading} searchPlaceholder="Cari no. transfer..." emptyMessage="Belum ada mutasi antar gudang." />
      </div>

      {creating && (
        <Modal
          title="Buat Mutasi Antar Gudang"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="transfer-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan sebagai Draft'}
              </button>
            </>
          }
        >
          <form id="transfer-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Dari Gudang</label>
                <select
                  className="form-select"
                  value={form.from_warehouse_id}
                  onChange={(e) => setForm({ ...form, from_warehouse_id: e.target.value })}
                  required
                >
                  <option value="">Pilih gudang...</option>
                  {warehouses.map((wh) => (
                    <option key={wh.id} value={wh.id}>{wh.code} - {wh.name}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Ke Gudang</label>
                <select
                  className="form-select"
                  value={form.to_warehouse_id}
                  onChange={(e) => setForm({ ...form, to_warehouse_id: e.target.value })}
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
                  value={form.transfer_date}
                  onChange={(e) => setForm({ ...form, transfer_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
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
                <label className="form-label mb-0">Baris Produk</label>
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
                      <th style={{ width: 100 }}>Qty</th>
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
                            value={l.quantity}
                            onChange={(e) => updateLine(i, { quantity: e.target.value })}
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
    </div>
  )
}

export default StockTransfersPage
