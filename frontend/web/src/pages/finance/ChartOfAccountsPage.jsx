import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { account_code: '', account_name: '', account_type: 'ASSET' }
const ACCOUNT_TYPES = ['ASSET', 'LIABILITY', 'EQUITY', 'REVENUE', 'EXPENSE']

const TYPE_BADGE = {
  ASSET: 'text-bg-primary',
  LIABILITY: 'text-bg-danger',
  EQUITY: 'text-bg-info',
  REVENUE: 'text-bg-success',
  EXPENSE: 'text-bg-warning',
}

function ChartOfAccountsPage() {
  const { companyId } = useCompany()
  const [accounts, setAccounts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadAccounts(cid) {
    setLoading(true)
    apiClient
      .get('/api/finance/accounts', { params: { company_id: cid } })
      .then(({ data }) => setAccounts(data))
      .catch(() => setError('Gagal memuat chart of accounts. Pastikan finance-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadAccounts(companyId)
  }, [companyId])

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/finance/accounts', { ...form, company_id: companyId })
      setCreating(false)
      setForm(emptyForm)
      loadAccounts(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat account')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'account_code', label: 'Code', render: (a) => <code>{a.account_code}</code> },
    { key: 'account_name', label: 'Nama Account', maxWidth: 260 },
    {
      key: 'account_type',
      label: 'Tipe',
      render: (a) => <span className={`badge ${TYPE_BADGE[a.account_type] ?? 'text-bg-secondary'}`}>{a.account_type}</span>,
    },
    {
      key: 'is_posting',
      label: 'Posting',
      cellClassName: 'text-secondary small',
      render: (a) => (a.is_posting ? 'Ya' : 'Tidak'),
      sortValue: (a) => (a.is_posting ? 1 : 0),
    },
    {
      key: 'is_active',
      label: 'Status',
      render: (a) => (
        <span className={`badge ${a.is_active ? 'text-bg-success' : 'text-bg-secondary'}`}>{a.is_active ? 'Aktif' : 'Nonaktif'}</span>
      ),
      sortValue: (a) => (a.is_active ? 1 : 0),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Chart of Accounts</h2>
          <div className="text-secondary small">Master akun untuk General Ledger &amp; Invoices.</div>
        </div>
        <button
          type="button"
          className="btn btn-primary btn-sm"
          disabled={!companyId}
          onClick={() => setCreating(true)}
        >
          <i className="bi bi-plus-lg me-1" />
          Tambah Account
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={accounts}
          loading={loading}
          searchPlaceholder="Cari code atau nama account..."
          emptyMessage="Belum ada account. Tambahkan Chart of Accounts terlebih dahulu."
        />
      </div>

      {creating && (
        <Modal
          title="Tambah Account"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="account-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="account-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div>
              <label className="form-label">Account Code</label>
              <input
                type="text"
                className="form-control"
                placeholder="mis. 1101"
                value={form.account_code}
                onChange={(e) => setForm({ ...form, account_code: e.target.value })}
                required
              />
            </div>
            <div>
              <label className="form-label">Nama Account</label>
              <input
                type="text"
                className="form-control"
                placeholder="mis. Kas"
                value={form.account_name}
                onChange={(e) => setForm({ ...form, account_name: e.target.value })}
                required
              />
            </div>
            <div>
              <label className="form-label">Tipe</label>
              <select
                className="form-select"
                value={form.account_type}
                onChange={(e) => setForm({ ...form, account_type: e.target.value })}
              >
                {ACCOUNT_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default ChartOfAccountsPage
