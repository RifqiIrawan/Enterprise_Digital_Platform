import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyLine = { product_name: '', description: '', quantity: 1, unit_price: '' }
const emptyForm = {
  customer_id: '',
  quotation_date: new Date().toISOString().slice(0, 10),
  valid_until: '',
  tax_amount: '',
  notes: '',
  lines: [{ ...emptyLine }],
}

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  SENT: 'text-bg-info',
  ACCEPTED: 'text-bg-success',
  REJECTED: 'text-bg-danger',
  EXPIRED: 'text-bg-warning',
  CONVERTED: 'text-bg-primary',
}

function QuotationsPage() {
  const { companyId, branchId } = useCompany()
  const [customers, setCustomers] = useState([])
  const [quotations, setQuotations] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState(null)

  function loadQuotations(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/sales/quotations', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setQuotations(data))
      .catch(() => setError('Gagal memuat data quotation. Pastikan sales-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadQuotations(companyId, branchId)
    apiClient.get('/api/sales/customers', { params: { company_id: companyId } }).then(({ data }) => setCustomers(data))
  }, [companyId, branchId])

  const customerName = (id) => customers.find((c) => c.id === id)?.name ?? id

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
  const total = subtotal + (Number(form.tax_amount) || 0)

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/sales/quotations', {
        company_id: companyId,
        branch_id: branchId || null,
        customer_id: form.customer_id,
        quotation_date: form.quotation_date,
        valid_until: form.valid_until || null,
        tax_amount: Number(form.tax_amount) || 0,
        notes: form.notes,
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
      loadQuotations(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat quotation')
    } finally {
      setSaving(false)
    }
  }

  async function handleAction(id, action, confirmMsg) {
    if (confirmMsg && !window.confirm(confirmMsg)) return
    setActingId(id)
    try {
      await apiClient.post(`/api/sales/quotations/${id}/${action}`)
      loadQuotations(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal memproses quotation')
    } finally {
      setActingId(null)
    }
  }

  const columns = [
    { key: 'quotation_number', label: 'No. Quotation', render: (q) => <code>{q.quotation_number}</code> },
    { key: 'customer_id', label: 'Customer', render: (q) => customerName(q.customer_id), sortValue: (q) => customerName(q.customer_id) },
    {
      key: 'quotation_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (q) => new Date(q.quotation_date).toLocaleDateString('id-ID'),
    },
    {
      key: 'total_amount',
      label: 'Total',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (q) => formatMoney(q.total_amount),
    },
    {
      key: 'status',
      label: 'Status',
      render: (q) => <span className={`badge ${STATUS_BADGE[q.status] ?? 'text-bg-secondary'}`}>{q.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (q) => (
        <div className="d-flex gap-1 justify-content-end">
          {q.status === 'DRAFT' && (
            <button type="button" className="btn btn-sm btn-outline-info" disabled={actingId === q.id} onClick={() => handleAction(q.id, 'send')}>
              Kirim
            </button>
          )}
          {q.status === 'SENT' && (
            <>
              <button type="button" className="btn btn-sm btn-outline-success" disabled={actingId === q.id} onClick={() => handleAction(q.id, 'accept')}>
                Accept
              </button>
              <button type="button" className="btn btn-sm btn-outline-danger" disabled={actingId === q.id} onClick={() => handleAction(q.id, 'reject')}>
                Reject
              </button>
            </>
          )}
          {q.status === 'ACCEPTED' && (
            <button
              type="button"
              className="btn btn-sm btn-outline-primary"
              disabled={actingId === q.id}
              onClick={() => handleAction(q.id, 'convert', 'Konversi quotation ini menjadi Sales Order?')}
            >
              Jadikan SO
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
          <h2 className="edp-page-title">Quotations</h2>
          <div className="text-secondary small">Penawaran ke customer, bisa dikonversi menjadi Sales Order setelah diterima.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={() => setCreating(true)}>
          <i className="bi bi-plus-lg me-1" />
          Buat Quotation
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={quotations}
          loading={loading}
          searchPlaceholder="Cari no. quotation..."
          emptyMessage="Belum ada quotation."
        />
      </div>

      {creating && (
        <Modal
          title="Buat Quotation"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="quotation-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan sebagai Draft'}
              </button>
            </>
          }
        >
          <form id="quotation-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-12">
                <label className="form-label">Customer</label>
                <select
                  className="form-select"
                  value={form.customer_id}
                  onChange={(e) => setForm({ ...form, customer_id: e.target.value })}
                  required
                >
                  <option value="">Pilih customer...</option>
                  {customers.map((c) => (
                    <option key={c.id} value={c.id}>{c.customer_code} - {c.name}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Quotation</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.quotation_date}
                  onChange={(e) => setForm({ ...form, quotation_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Berlaku Sampai</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.valid_until}
                  onChange={(e) => setForm({ ...form, valid_until: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Pajak (opsional)</label>
                <input
                  type="number"
                  className="form-control"
                  min="0"
                  value={form.tax_amount}
                  onChange={(e) => setForm({ ...form, tax_amount: e.target.value })}
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
                <label className="form-label mb-0">Baris Quotation</label>
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
                      <td className="fw-semibold text-nowrap">Subtotal</td>
                      <td className="fw-semibold">{formatMoney(subtotal)}</td>
                    </tr>
                    <tr>
                      <td colSpan={3}></td>
                      <td className="fw-semibold text-nowrap">Total</td>
                      <td className="fw-semibold">{formatMoney(total)}</td>
                    </tr>
                  </tfoot>
                </table>
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default QuotationsPage
