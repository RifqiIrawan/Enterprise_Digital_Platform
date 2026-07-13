import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { product_id: '', movement_type: 'IN', quantity: '', notes: '', movement_date: new Date().toISOString().slice(0, 10) }

const MOVEMENT_BADGE = {
  IN: 'text-bg-success',
  OUT: 'text-bg-danger',
}

const REFERENCE_LABEL = {
  PURCHASE_ORDER: 'Purchase Order',
  SALES_ORDER: 'Sales Order',
  TRANSFER: 'Mutasi Antar Gudang',
  OPNAME: 'Stock Opname',
  MANUAL: 'Manual',
}

function StockPage() {
  const { companyId, branchId } = useCompany()
  const [warehouses, setWarehouses] = useState([])
  const [products, setProducts] = useState([])
  const [warehouseId, setWarehouseId] = useState('')
  const [balances, setBalances] = useState([])
  const [movements, setMovements] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadStock(cid, whId, bid) {
    setLoading(true)
    Promise.all([
      // Stock balances aren't branch-scoped (no branch_id column on
      // stock_balances) -- only the movement history is.
      apiClient.get('/api/warehouse/stock', { params: { company_id: cid, warehouse_id: whId } }),
      apiClient.get('/api/warehouse/stock-movements', { params: { company_id: cid, warehouse_id: whId, branch_id: bid } }),
    ])
      .then(([stockRes, movementsRes]) => {
        setBalances(stockRes.data)
        setMovements(movementsRes.data)
      })
      .catch(() => setError('Gagal memuat data stok. Pastikan warehouse-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    apiClient.get('/api/warehouse/products', { params: { company_id: companyId } }).then(({ data }) => setProducts(data))
    apiClient.get('/api/warehouse/warehouses', { params: { company_id: companyId } }).then(({ data }) => {
      setWarehouses(data)
      const whId = data[0]?.id ?? ''
      setWarehouseId(whId)
      if (whId) loadStock(companyId, whId, branchId)
      else setLoading(false)
    })
  }, [companyId, branchId])

  function changeWarehouse(whId) {
    setWarehouseId(whId)
    if (companyId && whId) loadStock(companyId, whId, branchId)
  }

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
      await apiClient.post('/api/warehouse/stock-movements', {
        company_id: companyId,
        branch_id: branchId || null,
        warehouse_id: warehouseId,
        product_id: form.product_id,
        movement_type: form.movement_type,
        quantity: Number(form.quantity) || 0,
        notes: form.notes,
        movement_date: form.movement_date,
      })
      setCreating(false)
      loadStock(companyId, warehouseId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal mencatat mutasi stok')
    } finally {
      setSaving(false)
    }
  }

  const balanceColumns = [
    { key: 'product_sku', label: 'SKU', render: (b) => <code>{b.product_sku}</code> },
    { key: 'product_name', label: 'Produk', render: (b) => b.product_name },
    {
      key: 'quantity',
      label: 'Saldo',
      className: 'text-end',
      cellClassName: 'text-end fw-semibold',
      render: (b) => `${b.quantity} ${b.product_unit}`,
    },
  ]

  const movementColumns = [
    {
      key: 'movement_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (m) => new Date(m.movement_date).toLocaleDateString('id-ID'),
    },
    { key: 'product_name', label: 'Produk', render: (m) => m.product_name },
    {
      key: 'movement_type',
      label: 'Tipe',
      render: (m) => <span className={`badge ${MOVEMENT_BADGE[m.movement_type] ?? 'text-bg-secondary'}`}>{m.movement_type}</span>,
    },
    { key: 'quantity', label: 'Qty', className: 'text-end', cellClassName: 'text-end' },
    {
      key: 'reference_type',
      label: 'Sumber',
      render: (m) => REFERENCE_LABEL[m.reference_type] ?? m.reference_type,
    },
    { key: 'notes', label: 'Catatan', cellClassName: 'text-secondary small', maxWidth: 200 },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between flex-wrap gap-2">
        <div>
          <h2 className="edp-page-title">Stok Gudang</h2>
          <div className="text-secondary small">Saldo stok per gudang beserta histori mutasinya.</div>
        </div>
        <div className="d-flex gap-2">
          <select className="form-select form-select-sm" style={{ width: 220 }} value={warehouseId} onChange={(e) => changeWarehouse(e.target.value)}>
            {warehouses.length === 0 && <option value="">Belum ada gudang</option>}
            {warehouses.map((wh) => (
              <option key={wh.id} value={wh.id}>{wh.code} - {wh.name}</option>
            ))}
          </select>
          <button type="button" className="btn btn-primary btn-sm" disabled={!warehouseId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Catat Mutasi Manual
          </button>
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {warehouses.length === 0 && !loading && !error && (
        <div className="alert alert-warning py-2 small mb-0">Belum ada gudang. Tambahkan data gudang dulu di menu Master Gudang.</div>
      )}

      <div className="card p-3">
        <h6 className="mb-2">Saldo Stok</h6>
        <DataTable columns={balanceColumns} data={balances} loading={loading} searchPlaceholder="Cari produk..." emptyMessage="Belum ada saldo stok di gudang ini." />
      </div>

      <div className="card p-3">
        <h6 className="mb-2">Histori Mutasi</h6>
        <DataTable columns={movementColumns} data={movements} loading={loading} searchPlaceholder="Cari produk atau catatan..." emptyMessage="Belum ada mutasi stok di gudang ini." />
      </div>

      {creating && (
        <Modal
          title="Catat Mutasi Stok Manual"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="movement-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="movement-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div>
              <label className="form-label">Produk</label>
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
            <div className="row g-3">
              <div className="col-4">
                <label className="form-label">Tipe</label>
                <select
                  className="form-select"
                  value={form.movement_type}
                  onChange={(e) => setForm({ ...form, movement_type: e.target.value })}
                >
                  <option value="IN">Masuk (IN)</option>
                  <option value="OUT">Keluar (OUT)</option>
                </select>
              </div>
              <div className="col-4">
                <label className="form-label">Qty</label>
                <input
                  type="number"
                  className="form-control"
                  value={form.quantity}
                  onChange={(e) => setForm({ ...form, quantity: e.target.value })}
                  min="0"
                  required
                />
              </div>
              <div className="col-4">
                <label className="form-label">Tanggal</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.movement_date}
                  onChange={(e) => setForm({ ...form, movement_date: e.target.value })}
                  required
                />
              </div>
            </div>
            <div>
              <label className="form-label">Catatan</label>
              <input
                type="text"
                className="form-control"
                value={form.notes}
                onChange={(e) => setForm({ ...form, notes: e.target.value })}
                placeholder="mis. stok awal, koreksi manual"
              />
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default StockPage
