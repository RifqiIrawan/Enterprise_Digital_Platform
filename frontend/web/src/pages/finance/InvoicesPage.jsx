import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyLine = { account_id: '', description: '', quantity: 1, unit_price: '' }
const emptyForm = {
  invoice_type: 'AR',
  partner_name: '',
  invoice_date: new Date().toISOString().slice(0, 10),
  due_date: '',
  control_account_id: '',
  tax_account_id: '',
  tax_amount: '',
  lines: [{ ...emptyLine }],
}

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 2 }).format(n ?? 0)
}

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  POSTED: 'text-bg-success',
  PARTIALLY_PAID: 'text-bg-warning',
  PAID: 'text-bg-primary',
  CANCELLED: 'text-bg-danger',
}

function invoiceColumns(postingId, handlePost) {
  return [
    { key: 'invoice_number', label: 'No. Invoice', render: (inv) => <code>{inv.invoice_number}</code> },
    {
      key: 'invoice_type',
      label: 'Tipe',
      render: (inv) => (
        <span className={`badge ${inv.invoice_type === 'AR' ? 'text-bg-success' : 'text-bg-danger'}`}>{inv.invoice_type}</span>
      ),
    },
    { key: 'partner_name', label: 'Partner', maxWidth: 200 },
    {
      key: 'invoice_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (inv) => new Date(inv.invoice_date).toLocaleDateString('id-ID'),
    },
    {
      key: 'total_amount',
      label: 'Total',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (inv) => formatMoney(inv.total_amount),
    },
    {
      key: 'status',
      label: 'Status',
      render: (inv) => <span className={`badge ${STATUS_BADGE[inv.status] ?? 'text-bg-secondary'}`}>{inv.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (inv) =>
        inv.status === 'DRAFT' && (
          <button
            type="button"
            className="btn btn-sm btn-outline-success"
            disabled={postingId === inv.id}
            onClick={() => handlePost(inv.id)}
          >
            {postingId === inv.id ? 'Posting...' : 'Post'}
          </button>
        ),
    },
  ]
}

function InvoicesPage() {
  const { companyId, branchId } = useCompany()
  const [accounts, setAccounts] = useState([])
  const [invoices, setInvoices] = useState([])
  const [typeFilter, setTypeFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [postingId, setPostingId] = useState(null)

  function loadInvoices(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/finance/invoices', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setInvoices(data))
      .catch(() => setError('Gagal memuat invoice. Pastikan finance-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadInvoices(companyId, branchId)
    apiClient.get('/api/finance/accounts', { params: { company_id: companyId } }).then(({ data }) => setAccounts(data))
  }, [companyId, branchId])

  const filtered = typeFilter === 'all' ? invoices : invoices.filter((i) => i.invoice_type === typeFilter)

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
      await apiClient.post('/api/finance/invoices', {
        company_id: companyId,
        branch_id: branchId || null,
        invoice_type: form.invoice_type,
        partner_name: form.partner_name,
        invoice_date: form.invoice_date,
        due_date: form.due_date || null,
        control_account_id: form.control_account_id,
        tax_account_id: form.tax_account_id || null,
        tax_amount: Number(form.tax_amount) || 0,
        lines: form.lines
          .filter((l) => l.account_id && l.description)
          .map((l) => ({
            account_id: l.account_id,
            description: l.description,
            quantity: Number(l.quantity) || 0,
            unit_price: Number(l.unit_price) || 0,
          })),
      })
      setCreating(false)
      setForm(emptyForm)
      loadInvoices(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat invoice')
    } finally {
      setSaving(false)
    }
  }

  async function handlePost(id) {
    if (!window.confirm('Posting invoice ini? Jurnal GL akan otomatis dibuat.')) return
    setPostingId(id)
    try {
      await apiClient.post(`/api/finance/invoices/${id}/post`)
      loadInvoices(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal posting invoice')
    } finally {
      setPostingId(null)
    }
  }

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Invoices</h2>
          <div className="text-secondary small">Invoice AR (piutang customer) &amp; AP (hutang vendor).</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={() => setCreating(true)}>
          <i className="bi bi-plus-lg me-1" />
          Buat Invoice
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <ul className="nav edp-filter-tabs mb-2">
          {['all', 'AR', 'AP'].map((t) => (
            <li className="nav-item" key={t}>
              <button
                type="button"
                className={`edp-filter-tab ${typeFilter === t ? 'active' : ''}`}
                onClick={() => setTypeFilter(t)}
              >
                {t === 'all' ? 'Semua' : t}
              </button>
            </li>
          ))}
        </ul>
        <DataTable
          columns={invoiceColumns(postingId, handlePost)}
          data={filtered}
          loading={loading}
          searchPlaceholder="Cari no. invoice atau partner..."
          emptyMessage="Belum ada invoice."
        />
      </div>

      {creating && (
        <Modal
          title="Buat Invoice"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="invoice-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan sebagai Draft'}
              </button>
            </>
          }
        >
          <form id="invoice-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-4">
                <label className="form-label">Tipe</label>
                <select
                  className="form-select"
                  value={form.invoice_type}
                  onChange={(e) => setForm({ ...form, invoice_type: e.target.value })}
                >
                  <option value="AR">AR (Piutang)</option>
                  <option value="AP">AP (Hutang)</option>
                </select>
              </div>
              <div className="col-8">
                <label className="form-label">{form.invoice_type === 'AR' ? 'Nama Customer' : 'Nama Vendor'}</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.partner_name}
                  onChange={(e) => setForm({ ...form, partner_name: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Invoice</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.invoice_date}
                  onChange={(e) => setForm({ ...form, invoice_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Jatuh Tempo</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.due_date}
                  onChange={(e) => setForm({ ...form, due_date: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">{form.invoice_type === 'AR' ? 'Akun Piutang' : 'Akun Hutang'}</label>
                <select
                  className="form-select"
                  value={form.control_account_id}
                  onChange={(e) => setForm({ ...form, control_account_id: e.target.value })}
                  required
                >
                  <option value="">Pilih account...</option>
                  {accounts.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.account_code} - {a.account_name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Pajak (opsional)</label>
                <input
                  type="number"
                  className="form-control"
                  placeholder="Jumlah PPN"
                  value={form.tax_amount}
                  onChange={(e) => setForm({ ...form, tax_amount: e.target.value })}
                  min="0"
                />
              </div>
            </div>

            <div>
              <div className="d-flex justify-content-between align-items-center mb-2">
                <label className="form-label mb-0">Baris Invoice</label>
                <button type="button" className="btn btn-sm btn-outline-secondary" onClick={addLine}>
                  <i className="bi bi-plus-lg me-1" />
                  Baris
                </button>
              </div>
              <div className="table-responsive">
                <table className="table table-sm align-middle mb-0">
                  <thead>
                    <tr>
                      <th>Account</th>
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
                          <select
                            className="form-select form-select-sm"
                            value={l.account_id}
                            onChange={(e) => updateLine(i, { account_id: e.target.value })}
                          >
                            <option value="">Pilih...</option>
                            {accounts.map((a) => (
                              <option key={a.id} value={a.id}>
                                {a.account_code} - {a.account_name}
                              </option>
                            ))}
                          </select>
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

export default InvoicesPage
