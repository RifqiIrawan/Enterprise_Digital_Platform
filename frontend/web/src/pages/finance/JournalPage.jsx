import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'

const emptyLine = { account_id: '', debit_amount: '', credit_amount: '', description: '' }
const emptyForm = { entry_date: new Date().toISOString().slice(0, 10), description: '', lines: [{ ...emptyLine }, { ...emptyLine }] }

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 2 }).format(n ?? 0)
}

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  POSTED: 'text-bg-success',
  REVERSED: 'text-bg-danger',
}

function journalColumns(postingId, handlePost) {
  return [
    { key: 'entry_number', label: 'No. Jurnal', render: (e) => <code>{e.entry_number}</code> },
    {
      key: 'entry_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (e) => new Date(e.entry_date).toLocaleDateString('id-ID'),
    },
    { key: 'description', label: 'Deskripsi', maxWidth: 280 },
    { key: 'reference_type', label: 'Referensi', cellClassName: 'text-secondary small' },
    {
      key: 'total_debit',
      label: 'Debit',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (e) => formatMoney(e.total_debit),
    },
    {
      key: 'total_credit',
      label: 'Credit',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (e) => formatMoney(e.total_credit),
    },
    {
      key: 'status',
      label: 'Status',
      render: (e) => <span className={`badge ${STATUS_BADGE[e.status] ?? 'text-bg-secondary'}`}>{e.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (e) =>
        e.status === 'DRAFT' && (
          <button
            type="button"
            className="btn btn-sm btn-outline-success"
            disabled={postingId === e.id}
            onClick={() => handlePost(e.id)}
          >
            {postingId === e.id ? 'Posting...' : 'Post'}
          </button>
        ),
    },
  ]
}

function JournalPage() {
  const [companyId, setCompanyId] = useState('')
  const [accounts, setAccounts] = useState([])
  const [entries, setEntries] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [postingId, setPostingId] = useState(null)

  function loadEntries(cid) {
    setLoading(true)
    apiClient
      .get('/api/finance/journal-entries', { params: { company_id: cid } })
      .then(({ data }) => setEntries(data))
      .catch(() => setError('Gagal memuat jurnal. Pastikan finance-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) {
          loadEntries(cid)
          apiClient.get('/api/finance/accounts', { params: { company_id: cid } }).then(({ data }) => setAccounts(data))
        } else {
          setLoading(false)
        }
      })
      .catch(() => {
        setError('Gagal memuat data company.')
        setLoading(false)
      })
  }, [])

  function updateLine(index, patch) {
    setForm((f) => ({ ...f, lines: f.lines.map((l, i) => (i === index ? { ...l, ...patch } : l)) }))
  }

  function addLine() {
    setForm((f) => ({ ...f, lines: [...f.lines, { ...emptyLine }] }))
  }

  function removeLine(index) {
    setForm((f) => ({ ...f, lines: f.lines.filter((_, i) => i !== index) }))
  }

  const totalDebit = form.lines.reduce((sum, l) => sum + (Number(l.debit_amount) || 0), 0)
  const totalCredit = form.lines.reduce((sum, l) => sum + (Number(l.credit_amount) || 0), 0)
  const isBalanced = form.lines.length >= 2 && Math.abs(totalDebit - totalCredit) < 0.01 && totalDebit > 0

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/finance/journal-entries', {
        company_id: companyId,
        entry_date: form.entry_date,
        description: form.description,
        lines: form.lines
          .filter((l) => l.account_id)
          .map((l) => ({
            account_id: l.account_id,
            debit_amount: Number(l.debit_amount) || 0,
            credit_amount: Number(l.credit_amount) || 0,
            description: l.description,
          })),
      })
      setCreating(false)
      setForm(emptyForm)
      loadEntries(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat jurnal')
    } finally {
      setSaving(false)
    }
  }

  async function handlePost(id) {
    if (!window.confirm('Posting jurnal ini? Setelah posted, jurnal tidak bisa diubah lagi.')) return
    setPostingId(id)
    try {
      await apiClient.post(`/api/finance/journal-entries/${id}/post`)
      loadEntries(companyId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal posting jurnal')
    } finally {
      setPostingId(null)
    }
  }

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">General Ledger / Jurnal</h2>
          <div className="text-secondary small">Jurnal manual double-entry. Debit harus sama dengan credit.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={() => setCreating(true)}>
          <i className="bi bi-plus-lg me-1" />
          Buat Jurnal
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={journalColumns(postingId, handlePost)}
          data={entries}
          loading={loading}
          searchPlaceholder="Cari no. jurnal atau deskripsi..."
          emptyMessage="Belum ada jurnal."
        />
      </div>

      {creating && (
        <Modal
          title="Buat Jurnal"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="journal-form" className="btn btn-primary" disabled={saving || !isBalanced}>
                {saving ? 'Menyimpan...' : 'Simpan sebagai Draft'}
              </button>
            </>
          }
        >
          <form id="journal-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Tanggal</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.entry_date}
                  onChange={(e) => setForm({ ...form, entry_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Deskripsi</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                />
              </div>
            </div>

            <div>
              <div className="d-flex justify-content-between align-items-center mb-2">
                <label className="form-label mb-0">Baris Jurnal</label>
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
                      <th style={{ width: 110 }}>Debit</th>
                      <th style={{ width: 110 }}>Credit</th>
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
                            <option value="">Pilih account...</option>
                            {accounts.map((a) => (
                              <option key={a.id} value={a.id}>
                                {a.account_code} - {a.account_name}
                              </option>
                            ))}
                          </select>
                        </td>
                        <td>
                          <input
                            type="number"
                            className="form-control form-control-sm"
                            value={l.debit_amount}
                            onChange={(e) => updateLine(i, { debit_amount: e.target.value, credit_amount: '' })}
                            min="0"
                          />
                        </td>
                        <td>
                          <input
                            type="number"
                            className="form-control form-control-sm"
                            value={l.credit_amount}
                            onChange={(e) => updateLine(i, { credit_amount: e.target.value, debit_amount: '' })}
                            min="0"
                          />
                        </td>
                        <td>
                          {form.lines.length > 2 && (
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
                      <td className="text-end fw-semibold">Total</td>
                      <td className="fw-semibold">{formatMoney(totalDebit)}</td>
                      <td className="fw-semibold">{formatMoney(totalCredit)}</td>
                      <td></td>
                    </tr>
                  </tfoot>
                </table>
              </div>
              {!isBalanced && (
                <div className="text-danger small mt-1">Total debit dan credit harus sama sebelum bisa disimpan.</div>
              )}
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default JournalPage
