import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import DataTable from '../../components/common/DataTable.jsx'

const FACT_LABEL = {
  finance_journal_lines: 'Finance (General Ledger)',
  sales_order_lines: 'Sales',
  inventory_movements: 'Inventory',
}

function SyncStatusPage() {
  const [status, setStatus] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [syncing, setSyncing] = useState(false)
  const [syncResult, setSyncResult] = useState(null)

  function loadStatus() {
    setLoading(true)
    setError('')
    apiClient
      .get('/api/dw/sync/status')
      .then(({ data }) => setStatus(data))
      .catch(() => setError('Gagal memuat status sync. Pastikan dw-service dan ClickHouse aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    loadStatus()
  }, [])

  async function handleSyncNow() {
    setSyncing(true)
    setSyncResult(null)
    try {
      const { data } = await apiClient.post('/api/dw/sync')
      setSyncResult(data)
      loadStatus()
    } catch (err) {
      setError(err.response?.data?.error ?? 'Gagal menjalankan sync')
    } finally {
      setSyncing(false)
    }
  }

  const columns = [
    { key: 'fact', label: 'Fact Table', render: (s) => FACT_LABEL[s.fact] ?? s.fact },
    {
      key: 'row_count',
      label: 'Jumlah Baris',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (s) => new Intl.NumberFormat('id-ID').format(s.row_count),
    },
    {
      key: 'last_synced_at',
      label: 'Terakhir Disync',
      cellClassName: 'text-secondary small',
      render: (s) => (s.last_synced_at ? new Date(s.last_synced_at).toLocaleString('id-ID') : 'Belum pernah disync'),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Data Warehouse — Sync Status</h2>
          <div className="text-secondary small">
            ETL batch dari Finance, Sales, dan Inventory ke ClickHouse. Berjalan otomatis tiap beberapa menit, atau bisa dipicu manual di bawah ini.
          </div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={syncing} onClick={handleSyncNow}>
          <i className="bi bi-arrow-repeat me-1" />
          {syncing ? 'Menyinkronkan...' : 'Sync Now'}
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      {syncResult && (
        <div className="alert alert-info py-2 small mb-0">
          {syncResult.map((r) => (
            <div key={r.fact}>
              {FACT_LABEL[r.fact] ?? r.fact}: {r.error ? <span className="text-danger">{r.error}</span> : `${r.rows_synced} baris baru`}
            </div>
          ))}
        </div>
      )}

      <div className="card p-3">
        <DataTable columns={columns} data={status} loading={loading} searchPlaceholder="Cari fact table..." emptyMessage="Belum ada data." />
      </div>
    </div>
  )
}

export default SyncStatusPage
