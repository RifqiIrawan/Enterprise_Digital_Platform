import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyLine = { product_id: '', unit_price: '', quantity: 1 }
const emptyForm = {
  customer_name: '',
  customer_email: '',
  shipping_address: '',
  order_date: new Date().toISOString().slice(0, 10),
  notes: '',
  lines: [{ ...emptyLine }],
}

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

const STATUS_BADGE = {
  PENDING: 'text-bg-secondary',
  PAID: 'text-bg-info',
  SHIPPED: 'text-bg-warning',
  DELIVERED: 'text-bg-success',
  CANCELLED: 'text-bg-danger',
}

function OrdersPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [products, setProducts] = useState([])
  const [warehouses, setWarehouses] = useState([])
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState(null)

  const [shippingOrder, setShippingOrder] = useState(null)
  const [shipForm, setShipForm] = useState({ warehouse_id: '' })
  const [shipError, setShipError] = useState('')
  const [shipSaving, setShipSaving] = useState(false)

  const [viewingOrder, setViewingOrder] = useState(null)
  const [viewError, setViewError] = useState('')

  function loadOrders(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/ecommerce/orders', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setOrders(data))
      .catch(() => setError('Gagal memuat data order. Pastikan ecommerce-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadOrders(companyId, branchId)
    apiClient.get('/api/warehouse/products', { params: { company_id: companyId } }).then(({ data }) => setProducts(data))
    apiClient.get('/api/warehouse/warehouses', { params: { company_id: companyId } }).then(({ data }) => setWarehouses(data))
  }, [companyId, branchId])

  function updateLine(index, patch) {
    setForm((f) => ({ ...f, lines: f.lines.map((l, i) => (i === index ? { ...l, ...patch } : l)) }))
  }

  function addLine() {
    setForm((f) => ({ ...f, lines: [...f.lines, { ...emptyLine }] }))
  }

  function removeLine(index) {
    setForm((f) => ({ ...f, lines: f.lines.filter((_, i) => i !== index) }))
  }

  const subtotal = form.lines.reduce((sum, l) => sum + (Number(l.quantity) || 0) * (Number(l.unit_price) || 0), 0)

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/ecommerce/orders', {
        company_id: companyId,
        branch_id: branchId || null,
        customer_name: form.customer_name,
        customer_email: form.customer_email,
        shipping_address: form.shipping_address,
        order_date: form.order_date,
        notes: form.notes,
        lines: form.lines
          .filter((l) => l.product_id)
          .map((l) => {
            const p = products.find((pr) => pr.id === l.product_id)
            return {
              product_id: l.product_id,
              product_sku: p?.sku ?? '',
              product_name: p?.name ?? '',
              unit_price: Number(l.unit_price) || 0,
              quantity: Number(l.quantity) || 0,
            }
          }),
      })
      setCreating(false)
      setForm(emptyForm)
      loadOrders(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat order')
    } finally {
      setSaving(false)
    }
  }

  async function handleAction(id, action) {
    setActingId(id)
    try {
      await apiClient.post(`/api/ecommerce/orders/${id}/${action}`)
      loadOrders(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal memproses order')
    } finally {
      setActingId(null)
    }
  }

  function openShip(order) {
    setShippingOrder(order)
    setShipForm({ warehouse_id: '' })
    setShipError('')
  }

  async function handleShip(e) {
    e.preventDefault()
    setShipSaving(true)
    setShipError('')
    try {
      await apiClient.post(`/api/ecommerce/orders/${shippingOrder.id}/ship`, shipForm)
      setShippingOrder(null)
      loadOrders(companyId, branchId)
    } catch (err) {
      setShipError(err.response?.data?.error ?? 'Gagal mencatat pengiriman')
    } finally {
      setShipSaving(false)
    }
  }

  function openView(order) {
    setViewingOrder({ ...order, items: null })
    setViewError('')
    apiClient
      .get(`/api/ecommerce/orders/${order.id}`)
      .then(({ data }) => setViewingOrder(data))
      .catch((err) => setViewError(err.response?.data?.error ?? 'Gagal memuat baris order'))
  }

  const columns = [
    { key: 'order_number', label: 'No. Order', render: (o) => <code>{o.order_number}</code> },
    { key: 'customer_name', label: 'Customer' },
    {
      key: 'order_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (o) => new Date(o.order_date).toLocaleDateString('id-ID'),
    },
    {
      key: 'total_amount',
      label: 'Total',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (o) => formatMoney(o.total_amount),
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
          <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openView(o)}>
            Lihat Item
          </button>
          {can('update') && o.status === 'PENDING' && (
            <button type="button" className="btn btn-sm btn-outline-info" disabled={actingId === o.id} onClick={() => handleAction(o.id, 'pay')}>
              Pay
            </button>
          )}
          {can('approve') && o.status === 'PAID' && (
            <button type="button" className="btn btn-sm btn-outline-warning" disabled={actingId === o.id} onClick={() => openShip(o)}>
              Ship
            </button>
          )}
          {can('update') && o.status === 'SHIPPED' && (
            <button type="button" className="btn btn-sm btn-outline-success" disabled={actingId === o.id} onClick={() => handleAction(o.id, 'deliver')}>
              Deliver
            </button>
          )}
          {can('update') && (o.status === 'PENDING' || o.status === 'PAID') && (
            <button type="button" className="btn btn-sm btn-outline-danger" disabled={actingId === o.id} onClick={() => handleAction(o.id, 'cancel')}>
              Cancel
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
          <h2 className="edp-page-title">Orders</h2>
          <div className="text-secondary small">Order online (checkout) yang produknya diambil dari katalog warehouse-service.</div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={() => setCreating(true)}>
            <i className="bi bi-plus-lg me-1" />
            Buat Order
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={orders} loading={loading} searchPlaceholder="Cari no. order..." emptyMessage="Belum ada order." />
      </div>

      {creating && (
        <Modal
          title="Buat Order"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="order-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="order-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Nama Customer</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.customer_name}
                  onChange={(e) => setForm({ ...form, customer_name: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Email Customer</label>
                <input
                  type="email"
                  className="form-control"
                  value={form.customer_email}
                  onChange={(e) => setForm({ ...form, customer_email: e.target.value })}
                />
              </div>
              <div className="col-8">
                <label className="form-label">Alamat Pengiriman</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.shipping_address}
                  onChange={(e) => setForm({ ...form, shipping_address: e.target.value })}
                />
              </div>
              <div className="col-4">
                <label className="form-label">Tanggal Order</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.order_date}
                  onChange={(e) => setForm({ ...form, order_date: e.target.value })}
                  required
                />
              </div>
            </div>

            <div>
              <div className="d-flex justify-content-between align-items-center mb-2">
                <label className="form-label mb-0">Baris Keranjang</label>
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
                      <th style={{ width: 70 }}>Qty</th>
                      <th style={{ width: 110 }}>Harga</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {form.lines.map((l, i) => (
                      <tr key={i}>
                        <td>
                          <select className="form-select form-select-sm" value={l.product_id} onChange={(e) => updateLine(i, { product_id: e.target.value })}>
                            <option value="">Pilih produk...</option>
                            {products.map((p) => (
                              <option key={p.id} value={p.id}>
                                {p.sku} - {p.name}
                              </option>
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
                          <input
                            type="number"
                            className="form-control form-control-sm"
                            value={l.unit_price}
                            onChange={(e) => updateLine(i, { unit_price: e.target.value })}
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
                  <tfoot>
                    <tr>
                      <td></td>
                      <td className="fw-semibold text-nowrap">Total</td>
                      <td className="fw-semibold" colSpan={2}>
                        {formatMoney(subtotal)}
                      </td>
                    </tr>
                  </tfoot>
                </table>
              </div>
            </div>

            <div>
              <label className="form-label">Catatan</label>
              <textarea className="form-control" rows={2} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
            </div>
          </form>
        </Modal>
      )}

      {shippingOrder && (
        <Modal
          title={`Ship ${shippingOrder.order_number}`}
          onClose={() => setShippingOrder(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setShippingOrder(null)}>
                Batal
              </button>
              <button type="submit" form="ship-order-form" className="btn btn-primary" disabled={shipSaving}>
                {shipSaving ? 'Memproses...' : 'Ship'}
              </button>
            </>
          }
        >
          <form id="ship-order-form" onSubmit={handleShip} className="d-flex flex-column gap-3">
            {shipError && <div className="alert alert-danger py-2 small mb-0">{shipError}</div>}
            <div className="text-secondary small">Stok keluar akan dicatat otomatis di warehouse-service untuk seluruh baris order ini.</div>
            <div>
              <label className="form-label">Gudang Pengirim</label>
              <select
                className="form-select"
                value={shipForm.warehouse_id}
                onChange={(e) => setShipForm({ ...shipForm, warehouse_id: e.target.value })}
                required
              >
                <option value="">Pilih gudang...</option>
                {warehouses.map((wh) => (
                  <option key={wh.id} value={wh.id}>
                    {wh.code} - {wh.name}
                  </option>
                ))}
              </select>
            </div>
          </form>
        </Modal>
      )}

      {viewingOrder && (
        <Modal title={`Item ${viewingOrder.order_number ?? ''}`} onClose={() => setViewingOrder(null)}>
          {viewError && <div className="alert alert-danger py-2 small">{viewError}</div>}
          {!viewingOrder.items ? (
            <div className="text-secondary small">Memuat...</div>
          ) : (
            <div className="table-responsive">
              <table className="table table-sm align-middle mb-0">
                <thead>
                  <tr>
                    <th>SKU</th>
                    <th>Produk</th>
                    <th className="text-end">Qty</th>
                    <th className="text-end">Harga</th>
                    <th className="text-end">Subtotal</th>
                  </tr>
                </thead>
                <tbody>
                  {viewingOrder.items.map((item) => (
                    <tr key={item.id}>
                      <td>{item.product_sku}</td>
                      <td>{item.product_name}</td>
                      <td className="text-end">{item.quantity}</td>
                      <td className="text-end">{formatMoney(item.unit_price)}</td>
                      <td className="text-end">{formatMoney(item.line_total)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Modal>
      )}
    </div>
  )
}

export default OrdersPage
