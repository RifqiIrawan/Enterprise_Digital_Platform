import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

const METHOD_LABEL = { STRAIGHT_LINE: 'Garis Lurus', DECLINING_BALANCE: 'Saldo Menurun' }

function currentPeriod() {
  return new Date().toISOString().slice(0, 7)
}

function DepreciationPage() {
  const { companyId } = useCompany()
  const { can } = usePagePermission()
  const [runs, setRuns] = useState([])
  const [accounts, setAccounts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [period, setPeriod] = useState(currentPeriod())
  const [calculating, setCalculating] = useState(false)

  const [detail, setDetail] = useState(null)
  const [detailError, setDetailError] = useState('')
  const [postForm, setPostForm] = useState({ expense_account_id: '', accumulated_depreciation_account_id: '' })
  const [busy, setBusy] = useState(false)

  function loadRuns(cid) {
    setLoading(true)
    apiClient
      .get('/api/asset/depreciation-runs', { params: { company_id: cid } })
      .then(({ data }) => setRuns(data))
      .catch(() => setError('Gagal memuat data penyusutan. Pastikan asset-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadRuns(companyId)
    apiClient.get('/api/finance/accounts', { params: { company_id: companyId } }).then(({ data }) => setAccounts(data))
  }, [companyId])

  async function handleCalculate() {
    setCalculating(true)
    setError('')
    try {
      const { data } = await apiClient.post('/api/asset/depreciation-runs', { company_id: companyId, period })
      loadRuns(companyId)
      openDetail(data.id)
    } catch (err) {
      setError(err.response?.data?.error ?? 'Gagal menghitung penyusutan')
    } finally {
      setCalculating(false)
    }
  }

  async function openDetail(runID) {
    setDetailError('')
    setPostForm({ expense_account_id: '', accumulated_depreciation_account_id: '' })
    try {
      const { data } = await apiClient.get(`/api/asset/depreciation-runs/${runID}`)
      setDetail(data)
    } catch {
      setError('Gagal memuat rincian penyusutan.')
    }
  }

  async function handlePost(e) {
    e.preventDefault()
    setBusy(true)
    setDetailError('')
    try {
      await apiClient.post(`/api/asset/depreciation-runs/${detail.id}/post`, postForm)
      setDetail(null)
      loadRuns(companyId)
    } catch (err) {
      setDetailError(err.response?.data?.error ?? 'Gagal memposting penyusutan ke buku besar')
    } finally {
      setBusy(false)
    }
  }

  async function handleDelete() {
    setBusy(true)
    setDetailError('')
    try {
      await apiClient.delete(`/api/asset/depreciation-runs/${detail.id}`)
      setDetail(null)
      loadRuns(companyId)
    } catch (err) {
      setDetailError(err.response?.data?.error ?? 'Gagal menghapus perhitungan penyusutan')
    } finally {
      setBusy(false)
    }
  }

  const columns = [
    { key: 'period', label: 'Periode', render: (r) => <code>{r.period}</code> },
    { key: 'asset_count', label: 'Jumlah Aset', className: 'text-end', cellClassName: 'text-end' },
    {
      key: 'total_amount',
      label: 'Total Penyusutan',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (r) => formatMoney(r.total_amount),
      sortValue: (r) => r.total_amount,
    },
    {
      key: 'status',
      label: 'Status',
      render: (r) => <span className={`badge ${r.status === 'POSTED' ? 'text-bg-success' : 'text-bg-secondary'}`}>{r.status}</span>,
    },
    {
      key: 'posted_at',
      label: 'Diposting',
      cellClassName: 'text-secondary small',
      render: (r) => (r.posted_at ? new Date(r.posted_at).toLocaleString('id-ID') : '—'),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (r) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openDetail(r.id)}>
          Rincian
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Penyusutan</h2>
          <div className="text-secondary small">
            Dihitung per bulan untuk seluruh aset company, lalu diposting ke buku besar sebagai satu jurnal. Periode harus urut &mdash; yang sudah diposting tidak bisa dihitung mundur.
          </div>
        </div>
        {can('create') && (
          <div className="d-flex gap-2 align-items-center">
            <input type="month" className="form-control form-control-sm" value={period} onChange={(e) => setPeriod(e.target.value)} style={{ width: 150 }} />
            <button type="button" className="btn btn-primary btn-sm text-nowrap" disabled={!companyId || calculating} onClick={handleCalculate}>
              <i className="bi bi-calculator me-1" />
              {calculating ? 'Menghitung...' : 'Hitung Penyusutan'}
            </button>
          </div>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small mb-0">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={runs} loading={loading} searchPlaceholder="Cari periode..." emptyMessage="Belum ada perhitungan penyusutan." />
      </div>

      {detail && (
        <Modal
          title={`Penyusutan ${detail.period}`}
          onClose={() => setDetail(null)}
          footer={
            <>
              {detail.status === 'DRAFT' && can('delete') && (
                <button type="button" className="btn btn-outline-danger me-auto" disabled={busy} onClick={handleDelete}>
                  Hapus Perhitungan
                </button>
              )}
              <button type="button" className="btn btn-outline-secondary" onClick={() => setDetail(null)}>
                Tutup
              </button>
              {detail.status === 'DRAFT' && can('approve') && (
                <button type="submit" form="post-depreciation-form" className="btn btn-primary" disabled={busy}>
                  {busy ? 'Memposting...' : 'Post ke GL'}
                </button>
              )}
            </>
          }
        >
          <div className="d-flex flex-column gap-3">
            {detailError && <div className="alert alert-danger py-2 small mb-0">{detailError}</div>}

            <div className="row g-2 small">
              <div className="col-4">
                <div className="text-secondary">Jumlah aset</div>
                <div>{detail.asset_count}</div>
              </div>
              <div className="col-4">
                <div className="text-secondary">Total</div>
                <div className="fw-semibold">{formatMoney(detail.total_amount)}</div>
              </div>
              <div className="col-4">
                <div className="text-secondary">Status</div>
                <div>{detail.status}</div>
              </div>
            </div>

            <div className="table-responsive" style={{ maxHeight: 280 }}>
              <table className="table table-sm align-middle mb-0">
                <thead>
                  <tr>
                    <th>Aset</th>
                    <th>Metode</th>
                    <th className="text-end">Nilai Buku Awal</th>
                    <th className="text-end">Penyusutan</th>
                    <th className="text-end">Nilai Buku Akhir</th>
                  </tr>
                </thead>
                <tbody>
                  {detail.entries.map((e) => (
                    <tr key={e.id}>
                      <td>
                        <code>{e.asset_code}</code> <span className="text-secondary small">{e.asset_name}</span>
                      </td>
                      <td className="text-secondary small">{METHOD_LABEL[e.method] ?? e.method}</td>
                      <td className="text-end">{formatMoney(e.book_value_before)}</td>
                      <td className="text-end">{formatMoney(e.amount)}</td>
                      <td className="text-end">{formatMoney(e.book_value_after)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {detail.status === 'DRAFT' && can('approve') && (
              <form id="post-depreciation-form" onSubmit={handlePost} className="border-top pt-3 d-flex flex-column gap-3">
                <div className="text-secondary small">
                  Satu jurnal untuk seluruh periode: debit Beban Penyusutan {formatMoney(detail.total_amount)}, kredit Akumulasi Penyusutan sebesar yang sama, bertanggal akhir periode.
                </div>
                <div>
                  <label className="form-label">Akun Beban Penyusutan (Debit)</label>
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
                  <label className="form-label">Akun Akumulasi Penyusutan (Kredit)</label>
                  <select
                    className="form-select"
                    value={postForm.accumulated_depreciation_account_id}
                    onChange={(e) => setPostForm({ ...postForm, accumulated_depreciation_account_id: e.target.value })}
                    required
                  >
                    <option value="">Pilih account...</option>
                    {accounts.map((a) => (
                      <option key={a.id} value={a.id}>{a.account_code} - {a.account_name}</option>
                    ))}
                  </select>
                </div>
              </form>
            )}

            {detail.status === 'POSTED' && (
              <div className="text-secondary small border-top pt-3">
                Sudah diposting ke buku besar{detail.journal_entry_id ? ` (jurnal ${detail.journal_entry_id})` : ''}. Akumulasi penyusutan tiap aset sudah bertambah sebesar nilai di atas.
              </div>
            )}
          </div>
        </Modal>
      )}
    </div>
  )
}

export default DepreciationPage
