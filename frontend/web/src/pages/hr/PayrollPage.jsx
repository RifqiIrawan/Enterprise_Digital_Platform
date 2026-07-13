import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

function currentPeriod() {
  return new Date().toISOString().slice(0, 7)
}

const STATUS_BADGE = { DRAFT: 'text-bg-secondary', POSTED: 'text-bg-success' }

function PayrollPage() {
  const { companyId, branchId } = useCompany()
  const [accounts, setAccounts] = useState([])
  const [runs, setRuns] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [processing, setProcessing] = useState(false)
  const [processPeriod, setProcessPeriod] = useState(currentPeriod())
  const [processError, setProcessError] = useState('')
  const [processSaving, setProcessSaving] = useState(false)

  const [viewingRun, setViewingRun] = useState(null)
  const [viewLoading, setViewLoading] = useState(false)

  const [postingRun, setPostingRun] = useState(null)
  const [postForm, setPostForm] = useState({
    expense_account_id: '',
    salary_payable_account_id: '',
    tax_payable_account_id: '',
    bpjs_payable_account_id: '',
  })
  const [postError, setPostError] = useState('')
  const [postSaving, setPostSaving] = useState(false)

  function loadRuns(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/hr/payroll-runs', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setRuns(data))
      .catch(() => setError('Gagal memuat data payroll. Pastikan hr-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadRuns(companyId, branchId)
    apiClient.get('/api/finance/accounts', { params: { company_id: companyId } }).then(({ data }) => setAccounts(data))
  }, [companyId, branchId])

  async function handleProcess(e) {
    e.preventDefault()
    setProcessSaving(true)
    setProcessError('')
    try {
      await apiClient.post('/api/hr/payroll-runs', { company_id: companyId, branch_id: branchId || null, period: processPeriod })
      setProcessing(false)
      loadRuns(companyId, branchId)
    } catch (err) {
      setProcessError(err.response?.data?.error ?? 'Gagal memproses payroll')
    } finally {
      setProcessSaving(false)
    }
  }

  function openView(run) {
    setViewingRun({ ...run, details: [] })
    setViewLoading(true)
    apiClient
      .get(`/api/hr/payroll-runs/${run.id}`)
      .then(({ data }) => setViewingRun(data))
      .catch(() => setError('Gagal memuat detail payroll run'))
      .finally(() => setViewLoading(false))
  }

  function openPost(run) {
    setPostingRun(run)
    setPostForm({ expense_account_id: '', salary_payable_account_id: '', tax_payable_account_id: '', bpjs_payable_account_id: '' })
    setPostError('')
  }

  async function handlePost(e) {
    e.preventDefault()
    setPostSaving(true)
    setPostError('')
    try {
      await apiClient.post(`/api/hr/payroll-runs/${postingRun.id}/post`, postForm)
      setPostingRun(null)
      loadRuns(companyId, branchId)
    } catch (err) {
      setPostError(err.response?.data?.error ?? 'Gagal posting payroll ke GL')
    } finally {
      setPostSaving(false)
    }
  }

  const columns = [
    { key: 'period', label: 'Periode', render: (r) => <code>{r.period}</code> },
    { key: 'total_employees', label: 'Jml Karyawan', className: 'text-end', cellClassName: 'text-end' },
    {
      key: 'total_gross',
      label: 'Total Gross',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (r) => formatMoney(r.total_gross),
    },
    {
      key: 'total_deduction',
      label: 'Total Potongan',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (r) => formatMoney(r.total_deduction),
    },
    {
      key: 'total_net',
      label: 'Total Net',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (r) => formatMoney(r.total_net),
    },
    {
      key: 'status',
      label: 'Status',
      render: (r) => <span className={`badge ${STATUS_BADGE[r.status] ?? 'text-bg-secondary'}`}>{r.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (r) => (
        <div className="d-flex gap-1 justify-content-end">
          <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openView(r)}>
            Detail
          </button>
          {r.status === 'DRAFT' && (
            <button type="button" className="btn btn-sm btn-outline-success" onClick={() => openPost(r)}>
              Post ke GL
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
          <h2 className="edp-page-title">Payroll</h2>
          <div className="text-secondary small">Proses payroll bulanan &amp; posting ke General Ledger.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={() => setProcessing(true)}>
          <i className="bi bi-play-fill me-1" />
          Proses Payroll
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={runs}
          loading={loading}
          searchPlaceholder="Cari periode..."
          emptyMessage="Belum ada payroll run. Proses payroll untuk memulai."
        />
      </div>

      {processing && (
        <Modal
          title="Proses Payroll"
          onClose={() => setProcessing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setProcessing(false)}>
                Batal
              </button>
              <button type="submit" form="process-payroll-form" className="btn btn-primary" disabled={processSaving}>
                {processSaving ? 'Memproses...' : 'Proses'}
              </button>
            </>
          }
        >
          <form id="process-payroll-form" onSubmit={handleProcess} className="d-flex flex-column gap-3">
            {processError && <div className="alert alert-danger py-2 small mb-0">{processError}</div>}
            <div className="text-secondary small">
              Payroll akan dihitung untuk seluruh karyawan berstatus ACTIVE, pro-rata terhadap absensi bulan berjalan.
            </div>
            <div>
              <label className="form-label">Periode</label>
              <input
                type="month"
                className="form-control"
                value={processPeriod}
                onChange={(e) => setProcessPeriod(e.target.value)}
                required
              />
            </div>
          </form>
        </Modal>
      )}

      {viewingRun && (
        <Modal title={`Detail Payroll ${viewingRun.period}`} onClose={() => setViewingRun(null)}>
          {viewLoading ? (
            <div className="text-secondary small">Memuat...</div>
          ) : (
            <div className="table-responsive">
              <table className="table table-sm align-middle mb-0">
                <thead>
                  <tr>
                    <th>Karyawan</th>
                    <th className="text-end">Hadir</th>
                    <th className="text-end">Gross</th>
                    <th className="text-end">PPh21</th>
                    <th className="text-end">BPJS</th>
                    <th className="text-end">Net</th>
                  </tr>
                </thead>
                <tbody>
                  {viewingRun.details?.map((d) => (
                    <tr key={d.id}>
                      <td>{d.employee_name}</td>
                      <td className="text-end text-secondary small">{d.present_days}/{d.working_days}</td>
                      <td className="text-end">{formatMoney(d.gross_salary)}</td>
                      <td className="text-end">{formatMoney(d.pph21)}</td>
                      <td className="text-end">{formatMoney(d.bpjs_kesehatan_emp + d.bpjs_tk_jht_emp + d.bpjs_tk_jp_emp)}</td>
                      <td className="text-end fw-semibold">{formatMoney(d.net_salary)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Modal>
      )}

      {postingRun && (
        <Modal
          title={`Post Payroll ${postingRun.period} ke GL`}
          onClose={() => setPostingRun(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setPostingRun(null)}>
                Batal
              </button>
              <button type="submit" form="post-payroll-form" className="btn btn-primary" disabled={postSaving}>
                {postSaving ? 'Posting...' : 'Post'}
              </button>
            </>
          }
        >
          <form id="post-payroll-form" onSubmit={handlePost} className="d-flex flex-column gap-3">
            {postError && <div className="alert alert-danger py-2 small mb-0">{postError}</div>}
            <div className="text-secondary small">
              Jurnal GL akan dibuat otomatis: debit Beban Gaji sebesar total gross ({formatMoney(postingRun.total_gross)}),
              credit ke akun-akun di bawah ini.
            </div>
            <div>
              <label className="form-label">Akun Beban Gaji (Debit)</label>
              <select
                className="form-select"
                value={postForm.expense_account_id}
                onChange={(e) => setPostForm({ ...postForm, expense_account_id: e.target.value })}
                required
              >
                <option value="">Pilih account...</option>
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>{a.account_code} - {a.account_name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="form-label">Akun Hutang Gaji, Net ({formatMoney(postingRun.total_net)})</label>
              <select
                className="form-select"
                value={postForm.salary_payable_account_id}
                onChange={(e) => setPostForm({ ...postForm, salary_payable_account_id: e.target.value })}
                required
              >
                <option value="">Pilih account...</option>
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>{a.account_code} - {a.account_name}</option>
                ))}
              </select>
            </div>
            {postingRun.total_pph21 > 0 && (
              <div>
                <label className="form-label">Akun Hutang PPh21 ({formatMoney(postingRun.total_pph21)})</label>
                <select
                  className="form-select"
                  value={postForm.tax_payable_account_id}
                  onChange={(e) => setPostForm({ ...postForm, tax_payable_account_id: e.target.value })}
                  required
                >
                  <option value="">Pilih account...</option>
                  {accounts.map((a) => (
                    <option key={a.id} value={a.id}>{a.account_code} - {a.account_name}</option>
                  ))}
                </select>
              </div>
            )}
            {postingRun.total_bpjs > 0 && (
              <div>
                <label className="form-label">Akun Hutang BPJS ({formatMoney(postingRun.total_bpjs)})</label>
                <select
                  className="form-select"
                  value={postForm.bpjs_payable_account_id}
                  onChange={(e) => setPostForm({ ...postForm, bpjs_payable_account_id: e.target.value })}
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

export default PayrollPage
