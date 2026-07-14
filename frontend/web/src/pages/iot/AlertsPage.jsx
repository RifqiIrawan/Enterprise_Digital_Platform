import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const STATUS_BADGE = {
  OPEN: 'text-bg-danger',
  ACKNOWLEDGED: 'text-bg-warning',
  RESOLVED: 'text-bg-success',
}

const SEVERITY_BADGE = {
  MEDIUM: 'text-bg-warning',
  HIGH: 'text-bg-danger',
}

function AlertsPage() {
  const { companyId, branchId } = useCompany()
  const [alerts, setAlerts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [actingId, setActingId] = useState(null)

  function loadAlerts(cid, bid, status) {
    setLoading(true)
    apiClient
      .get('/api/iot/alerts', { params: { company_id: cid, branch_id: bid, status: status || undefined } })
      .then(({ data }) => setAlerts(data))
      .catch(() => setError('Gagal memuat data alert. Pastikan iot-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadAlerts(companyId, branchId, statusFilter)
  }, [companyId, branchId, statusFilter])

  async function handleAction(id, action) {
    setActingId(id)
    try {
      await apiClient.post(`/api/iot/alerts/${id}/${action}`)
      loadAlerts(companyId, branchId, statusFilter)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal memproses alert')
    } finally {
      setActingId(null)
    }
  }

  const columns = [
    { key: 'triggered_at', label: 'Waktu', cellClassName: 'text-secondary small', render: (a) => new Date(a.triggered_at).toLocaleString('id-ID') },
    { key: 'device_id', label: 'Device', render: (a) => `${a.device_code} - ${a.device_name}`, sortValue: (a) => a.device_code },
    {
      key: 'severity',
      label: 'Severity',
      render: (a) => <span className={`badge ${SEVERITY_BADGE[a.severity] ?? 'text-bg-secondary'}`}>{a.severity}</span>,
    },
    { key: 'message', label: 'Pesan', cellClassName: 'text-secondary small', maxWidth: 320 },
    {
      key: 'status',
      label: 'Status',
      render: (a) => <span className={`badge ${STATUS_BADGE[a.status] ?? 'text-bg-secondary'}`}>{a.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (a) => (
        <div className="d-flex gap-1 justify-content-end">
          {a.status === 'OPEN' && (
            <button type="button" className="btn btn-sm btn-outline-warning" disabled={actingId === a.id} onClick={() => handleAction(a.id, 'acknowledge')}>
              Acknowledge
            </button>
          )}
          {(a.status === 'OPEN' || a.status === 'ACKNOWLEDGED') && (
            <button type="button" className="btn btn-sm btn-outline-success" disabled={actingId === a.id} onClick={() => handleAction(a.id, 'resolve')}>
              Resolve
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
          <h2 className="edp-page-title">IoT Alerts</h2>
          <div className="text-secondary small">Alert ambang batas dari device numerik (TEMPERATURE/HUMIDITY/VIBRATION).</div>
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <div className="row g-2 mb-3">
          <div className="col-6 col-md-4">
            <select className="form-select form-select-sm" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
              <option value="">Semua status</option>
              <option value="OPEN">OPEN</option>
              <option value="ACKNOWLEDGED">ACKNOWLEDGED</option>
              <option value="RESOLVED">RESOLVED</option>
            </select>
          </div>
        </div>
        <DataTable columns={columns} data={alerts} loading={loading} searchPlaceholder="Cari device atau pesan..." emptyMessage="Belum ada alert." />
      </div>
    </div>
  )
}

export default AlertsPage
