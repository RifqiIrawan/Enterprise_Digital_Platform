import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyLine = { product_name: '', description: '', quantity: 1, unit_price: '' }
const emptyForm = { supplier_id: '', order_date: new Date().toISOString().slice(0, 10), lines: [{ ...emptyLine }] }

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  CONFIRMED: 'text-bg-info',
  RECEIVED: 'text-bg-warning',
  INVOICED: 'text-bg-success',
  CANCELLED: 'text-bg-danger',
}

function PurchaseOrdersPage() {
  const { companyId } = useCompany()
  const [suppliers, setSuppliers] = useState([])
  const [accounts, setAccounts] = useState([])
  const [warehouses, setWarehouses] = useState([])
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState(null)

  const [invoicingOrder, setInvoicingOrder] = useState(null)
  const [invoiceForm, setInvoiceForm] = useState({ expense_account_id: '', control_account_id: '', tax_account_id: '' })
  const [invoiceError, setInvoiceError] = useState('')
  const [invoiceSaving, setInvoiceSaving] = useState(false)

  const [receivingOrder, setReceivingOrder] = useState(null)
  const [receiveForm, setReceiveForm] = useState({ warehouse_id: '' })
  const [receiveError, setReceiveError] = useState('')
  const [receiveSaving, setReceiveSaving] = useState(false)

  function loadOrders(cid) {
    setLoading(true)
    apiClient
      .get('/api/purchasing/purchase-orders', { params: { company_id: cid } })
      .then(({ data }) => setOrders(data))
      .catch(() => setError('Gagal memuat data purchase order. Pastikan purchasing-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadOrders(companyId)
    apiClient.get('/api/purchasing/suppliers', { params: { company_id: companyId } }).then(({ data }) => setSuppliers(data))
    apiClient.get('/api/finance/accounts', { params: { company_id: companyId } }).then(({ data }) => setAccounts(data))
    apiClient.get('/api/warehouse/warehouses', { params: { company_id: companyId } }).then(({ data }) => setWarehouses(data))
  }, [companyId])

  const supplierName = (id) => suppliers.find((s) => s.id === id)?.name ?? id

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
      await apiClient.post('/api/purchasing/purchase-orders', {
        company_id: companyId,
        supplier_id: form.supplier_id,
        order_date: form.order_date,
        lines: form.lines
          .filter((l) => l.product_name)
          .map((l) => ({
            product_name: l.product_name,
            description: l.description,
            quantity: Number(l.quantity) || 0,
            unit_price: Number(l.unit_price) || 0,
          })),
      })
      setCreating(false)
      setForm(emptyForm)
      loadOrders(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat purchase order')
    } finally {
      setSaving(false)
    }
  }

  async function handleAction(id, action) {
    setActingId(id)
    try {
      await apiClient.post(`/api/purchasing/purchase-orders/${id}/${action}`)
      loadOrders(companyId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal memproses purchase order')
    } finally {
      setActingId(null)
    }
  }

  function openReceive(order) {
    setReceivingOrder(order)
    setReceiveForm({ warehouse_id: '' })
    setReceiveError('')
  }

  async function handleReceive(e) {
    e.preventDefault()
    setReceiveSaving(true)
    setReceiveError('')
    try {
      await apiClient.post(`/api/purchasing/purchase-orders/${receivingOrder.id}/receive`, receiveForm)
      setReceivingOrder(null)
      loadOrders(companyId)
    } catch (err) {
      setReceiveError(err.response?.data?.error ?? 'Gagal menerima barang')
    } finally {
      setReceiveSaving(false)
    }
  }

  function openInvoice(order) {
    setInvoicingOrder(order)
    setInvoiceForm({ expense_account_id: '', control_account_id: '', tax_account_id: '' })
    setInvoiceError('')
  }

  async function handleInvoice(e) {
    e.preventDefault()
    setInvoiceSaving(true)
    setInvoiceError('')
    try {
      await apiClient.post(`/api/purchasing/purchase-orders/${invoicingOrder.id}/invoice`, invoiceForm)
      setInvoicingOrder(null)
      loadOrders(companyId)
    } catch (err) {
      setInvoiceError(err.response?.data?.error ?? 'Gagal membuat invoice')
    } finally {
      setInvoiceSaving(false)
    }
  }

  const columns = [
    { key: 'po_number', label: 'No. PO', render: (o) => <code>{o.po_number}</code> },
    { key: 'supplier_id', label: 'Supplier', render: (o) => supplierName(o.supplier_id), sortValue: (o) => supplierName(o.supplier_id) },
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
          {o.status === 'DRAFT' && (
            <button type="button" className="btn btn-sm btn-outline-info" disabled={actingId === o.id} onClick={() => handleAction(o.id, 'confirm')}>
              Confirm
            </button>
          )}
          {o.status === 'CONFIRMED' && (
            <button type="button" className="btn btn-sm btn-outline-warning" disabled={actingId === o.id} onClick={() => openReceive(o)}>
              Terima Barang
            </button>
          )}
          {(o.status === 'CONFIRMED' || o.status === 'RECEIVED') && (
            <button type="button" className="btn btn-sm btn-outline-success" onClick={() => openInvoice(o)}>
              Buat Invoice
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
          <h2 className="edp-page-title">Purchase Orders</h2>
          <div className="text-secondary small">Order ke supplier, bisa dibuat langsung atau hasil konversi requisition.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={() => setCreating(true)}>
          <i className="bi bi-plus-lg me-1" />
          Buat Purchase Order
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={orders}
          loading={loading}
          searchPlaceholder="Cari no. purchase order..."
          emptyMessage="Belum ada purchase order."
        />
      </div>

      {creating && (
        <Modal
          title="Buat Purchase Order"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="po-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan sebagai Draft'}
              </button>
            </>
          }
        >
          <form id="po-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-8">
                <label className="form-label">Supplier</label>
                <select
                  className="form-select"
                  value={form.supplier_id}
                  onChange={(e) => setForm({ ...form, supplier_id: e.target.value })}
                  required
                >
                  <option value="">Pilih supplier...</option>
                  {suppliers.map((s) => (
                    <option key={s.id} value={s.id}>{s.supplier_code} - {s.name}</option>
                  ))}
                </select>
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
                <label className="form-label mb-0">Baris Order</label>
                <button type="button" className="btn btn-sm btn-outline-secondary" onClick={addLine}>
                  <i className="bi bi-plus-lg me-1" />
                  Baris
                </button>
              </div>
              <div className="table-responsive">
                <table className="table table-sm align-middle mb-0">
                  <thead>
                    <tr>
                      <th>Produk / Jasa</th>
                      <th>Deskripsi</th>
                      <th style={{ width: 70 }}>Qty</th>
                      <th style={{ width: 110 }}>Harga</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {form.lines.map((l, i) => (
                      <tr key={i}>
                        <td>
                          <input
                            type="text"
                            className="form-control form-control-sm"
                            value={l.product_name}
                            onChange={(e) => updateLine(i, { product_name: e.target.value })}
                          />
                        </td>
                        <td>
                          <input
                            type="text"
                            className="form-control form-control-sm"
                            value={l.description}
                            onChange={(e) => updateLine(i, { description: e.target.value })}
                          />
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
                      <td colSpan={3}></td>
                      <td className="fw-semibold text-nowrap">Total</td>
                      <td className="fw-semibold">{formatMoney(subtotal)}</td>
                    </tr>
                  </tfoot>
                </table>
              </div>
            </div>
          </form>
        </Modal>
      )}

      {receivingOrder && (
        <Modal
          title={`Terima Barang untuk ${receivingOrder.po_number}`}
          onClose={() => setReceivingOrder(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setReceivingOrder(null)}>
                Batal
              </button>
              <button type="submit" form="receive-po-form" className="btn btn-primary" disabled={receiveSaving}>
                {receiveSaving ? 'Memproses...' : 'Terima Barang'}
              </button>
            </>
          }
        >
          <form id="receive-po-form" onSubmit={handleReceive} className="d-flex flex-column gap-3">
            {receiveError && <div className="alert alert-danger py-2 small mb-0">{receiveError}</div>}
            <div className="text-secondary small">
              Stok masuk akan dicatat otomatis di warehouse-service untuk seluruh baris PO ini.
            </div>
            <div>
              <label className="form-label">Gudang Penerima</label>
              <select
                className="form-select"
                value={receiveForm.warehouse_id}
                onChange={(e) => setReceiveForm({ ...receiveForm, warehouse_id: e.target.value })}
                required
              >
                <option value="">Pilih gudang...</option>
                {warehouses.map((wh) => (
                  <option key={wh.id} value={wh.id}>{wh.code} - {wh.name}</option>
                ))}
              </select>
            </div>
          </form>
        </Modal>
      )}

      {invoicingOrder && (
        <Modal
          title={`Buat Invoice untuk ${invoicingOrder.po_number}`}
          onClose={() => setInvoicingOrder(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setInvoicingOrder(null)}>
                Batal
              </button>
              <button type="submit" form="invoice-po-form" className="btn btn-primary" disabled={invoiceSaving}>
                {invoiceSaving ? 'Membuat...' : 'Buat Invoice'}
              </button>
            </>
          }
        >
          <form id="invoice-po-form" onSubmit={handleInvoice} className="d-flex flex-column gap-3">
            {invoiceError && <div className="alert alert-danger py-2 small mb-0">{invoiceError}</div>}
            <div className="text-secondary small">
              Invoice AP akan dibuat &amp; diposting otomatis di finance-service sebesar {formatMoney(invoicingOrder.total_amount)}.
            </div>
            <div>
              <label className="form-label">Akun Hutang Usaha (Control Account)</label>
              <select
                className="form-select"
                value={invoiceForm.control_account_id}
                onChange={(e) => setInvoiceForm({ ...invoiceForm, control_account_id: e.target.value })}
                required
              >
                <option value="">Pilih account...</option>
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>{a.account_code} - {a.account_name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="form-label">Akun Beban (Expense)</label>
              <select
                className="form-select"
                value={invoiceForm.expense_account_id}
                onChange={(e) => setInvoiceForm({ ...invoiceForm, expense_account_id: e.target.value })}
                required
              >
                <option value="">Pilih account...</option>
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>{a.account_code} - {a.account_name}</option>
                ))}
              </select>
            </div>
            {invoicingOrder.tax_amount > 0 && (
              <div>
                <label className="form-label">Akun PPN Masukan ({formatMoney(invoicingOrder.tax_amount)})</label>
                <select
                  className="form-select"
                  value={invoiceForm.tax_account_id}
                  onChange={(e) => setInvoiceForm({ ...invoiceForm, tax_account_id: e.target.value })}
                  required
                >
                  <option value="">Pilih account...</option>
                  {accounts.map((a) => (
                    <option key={a.id} value={a.id}>{a.account_code} - {a.account_name}</option>
                  ))}
                </select>
              </div>
            )}
          </form>
        </Modal>
      )}
    </div>
  )
}

export default PurchaseOrdersPage
