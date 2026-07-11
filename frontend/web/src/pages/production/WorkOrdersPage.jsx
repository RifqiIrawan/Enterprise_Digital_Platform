import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'

const emptyForm = { bom_id: '', warehouse_id: '', quantity_planned: 1, planned_start_date: new Date().toISOString().slice(0, 10), planned_end_date: '', notes: '' }

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  IN_PROGRESS: 'text-bg-info',
  COMPLETED: 'text-bg-success',
  CANCELLED: 'text-bg-danger',
}

function WorkOrdersPage() {
  const [companyId, setCompanyId] = useState('')
  const [boms, setBoms] = useState([])
  const [warehouses, setWarehouses] = useState([])
  const [products, setProducts] = useState([])
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState(null)

  const [completingOrder, setCompletingOrder] = useState(null)
  const [completeQty, setCompleteQty] = useState(0)
  const [completeError, setCompleteError] = useState('')
  const [completeSaving, setCompleteSaving] = useState(false)

  function loadOrders(cid) {
    setLoading(true)
    apiClient
      .get('/api/production/work-orders', { params: { company_id: cid } })
      .then(({ data }) => setOrders(data))
      .catch(() => setError('Gagal memuat data work order. Pastikan production-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) {
          loadOrders(cid)
          apiClient.get('/api/production/boms', { params: { company_id: cid } }).then(({ data }) => setBoms(data.filter((b) => b.is_active)))
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

  const bomName = (id) => boms.find((b) => b.id === id)?.name ?? id
  const productName = (id) => {
    const p = products.find((p) => p.id === id)
    return p ? `${p.sku} - ${p.name}` : id
  }
  const warehouseName = (id) => warehouses.find((w) => w.id === id)?.name ?? id

  function openCreate() {
    setForm({ ...emptyForm })
    setFormError('')
    setCreating(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/production/work-orders', {
        company_id: companyId,
        bom_id: form.bom_id,
        warehouse_id: form.warehouse_id,
        quantity_planned: Number(form.quantity_planned) || 0,
        planned_start_date: form.planned_start_date,
        planned_end_date: form.planned_end_date || null,
        notes: form.notes,
      })
      setCreating(false)
      loadOrders(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat work order')
    } finally {
      setSaving(false)
    }
  }

  async function handleStart(id) {
    setActingId(id)
    try {
      await apiClient.post(`/api/production/work-orders/${id}/start`)
      loadOrders(companyId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal memulai work order')
    } finally {
      setActingId(null)
    }
  }

  function openComplete(o) {
    setCompletingOrder(o)
    setCompleteQty(o.quantity_planned)
    setCompleteError('')
  }

  async function handleComplete(e) {
    e.preventDefault()
    setCompleteSaving(true)
    setCompleteError('')
    try {
      await apiClient.post(`/api/production/work-orders/${completingOrder.id}/complete`, {
        quantity_produced: Number(completeQty) || 0,
      })
      setCompletingOrder(null)
      loadOrders(companyId)
    } catch (err) {
      setCompleteError(err.response?.data?.error ?? 'Gagal menyelesaikan work order')
    } finally {
      setCompleteSaving(false)
    }
  }

  const columns = [
    { key: 'wo_number', label: 'No. WO', render: (o) => <code>{o.wo_number}</code> },
    { key: 'product_id', label: 'Produk Jadi', render: (o) => productName(o.product_id), sortValue: (o) => productName(o.product_id) },
    { key: 'bom_id', label: 'BOM', render: (o) => bomName(o.bom_id), sortValue: (o) => bomName(o.bom_id) },
    { key: 'warehouse_id', label: 'Gudang', render: (o) => warehouseName(o.warehouse_id), sortValue: (o) => warehouseName(o.warehouse_id) },
    { key: 'quantity_planned', label: 'Rencana', className: 'text-end', cellClassName: 'text-end' },
    {
      key: 'quantity_produced',
      label: 'Hasil',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (o) => (o.quantity_produced != null ? o.quantity_produced : '—'),
    },
    {
      key: 'planned_start_date',
      label: 'Mulai',
      cellClassName: 'text-secondary small',
      render: (o) => new Date(o.planned_start_date).toLocaleDateString('id-ID'),
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
        <div className="d-flex gap-1 justify-content-end">
          {o.status === 'DRAFT' && (
            <button type="button" className="btn btn-sm btn-outline-info" disabled={actingId === o.id} onClick={() => handleStart(o.id)}>
              Mulai
            </button>
          )}
          {o.status === 'IN_PROGRESS' && (
            <button type="button" className="btn btn-sm btn-outline-success" onClick={() => openComplete(o)}>
              Selesaikan
            </button>
          )}
        </div>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Work Order</h2>
          <div className="text-secondary small">Jalankan produksi berdasarkan BOM; menyelesaikan WO otomatis memutasi stok komponen &amp; produk jadi.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId || boms.length === 0} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Buat Work Order
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {boms.length === 0 && !loading && !error && (
        <div className="alert alert-warning py-2 small mb-0">Belum ada BOM aktif. Buat BOM dulu di menu Bill of Material.</div>
      )}

      <div className="card p-3">
        <DataTable columns={columns} data={orders} loading={loading} searchPlaceholder="Cari no. work order..." emptyMessage="Belum ada work order." />
      </div>

      {creating && (
        <Modal
          title="Buat Work Order"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="wo-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan sebagai Draft'}
              </button>
            </>
          }
        >
          <form id="wo-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div>
              <label className="form-label">BOM</label>
              <select className="form-select" value={form.bom_id} onChange={(e) => setForm({ ...form, bom_id: e.target.value })} required>
                <option value="">Pilih BOM...</option>
                {boms.map((b) => (
                  <option key={b.id} value={b.id}>{b.bom_code} - {b.name}</option>
                ))}
              </select>
            </div>
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Gudang</label>
                <select className="form-select" value={form.warehouse_id} onChange={(e) => setForm({ ...form, warehouse_id: e.target.value })} required>
                  <option value="">Pilih gudang...</option>
                  {warehouses.map((wh) => (
                    <option key={wh.id} value={wh.id}>{wh.code} - {wh.name}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Qty Rencana</label>
                <input
                  type="number"
                  className="form-control"
                  value={form.quantity_planned}
                  onChange={(e) => setForm({ ...form, quantity_planned: e.target.value })}
                  min="0"
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Mulai</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.planned_start_date}
                  onChange={(e) => setForm({ ...form, planned_start_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Selesai (opsional)</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.planned_end_date}
                  onChange={(e) => setForm({ ...form, planned_end_date: e.target.value })}
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
          </form>
        </Modal>
      )}

      {completingOrder && (
        <Modal
          title={`Selesaikan ${completingOrder.wo_number}`}
          onClose={() => setCompletingOrder(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCompletingOrder(null)}>
                Batal
              </button>
              <button type="submit" form="complete-wo-form" className="btn btn-primary" disabled={completeSaving}>
                {completeSaving ? 'Memproses...' : 'Selesaikan'}
              </button>
            </>
          }
        >
          <form id="complete-wo-form" onSubmit={handleComplete} className="d-flex flex-column gap-3">
            {completeError && <div className="alert alert-danger py-2 small mb-0">{completeError}</div>}
            <div className="text-secondary small">
              Komponen sesuai BOM akan dikonsumsi (stock OUT) dan produk jadi akan ditambahkan (stock IN) di warehouse-service.
            </div>
            <div>
              <label className="form-label">Qty Hasil Produksi</label>
              <input
                type="number"
                className="form-control"
                value={completeQty}
                onChange={(e) => setCompleteQty(e.target.value)}
                min="0"
                required
              />
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default WorkOrdersPage
