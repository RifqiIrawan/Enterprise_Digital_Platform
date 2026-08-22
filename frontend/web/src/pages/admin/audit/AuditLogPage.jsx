import { useCallback, useEffect, useMemo, useState } from 'react'
import apiClient from '../../../services/apiClient.js'
import Modal from '../../../components/common/Modal.jsx'
import DataTable, { TruncatedText } from '../../../components/common/DataTable.jsx'
import { useCompany } from '../../../store/CompanyContext.jsx'
import { usePagePermission } from '../../../store/PermissionContext.jsx'

const ACTION_BADGE = {
  create: 'text-bg-success',
  update: 'text-bg-primary',
  delete: 'text-bg-danger',
  login: 'text-bg-secondary',
}

const emptyFilters = { event_type: '', entity_type: '', from: '', to: '', limit: '100' }

// Audit log dicatat dari event Kafka, jadi isinya bergantung pada audit-service
// yang hidup DAN Kafka yang jalan saat event terjadi. Halaman ini murni baca:
// tidak ada endpoint tulis/hapus di audit-service, dan memang tidak boleh ada --
// jejak audit yang bisa diedit dari UI kehilangan seluruh gunanya.
function AuditLogPage() {
  const { companyId } = useCompany()
  const { can } = usePagePermission()

  const [logs, setLogs] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [filters, setFilters] = useState(emptyFilters)
  const [detail, setDetail] = useState(null)

  const loadLogs = useCallback((cid, f) => {
    setLoading(true)
    const params = { company_id: cid || undefined, limit: f.limit || undefined }
    if (f.event_type) params.event_type = f.event_type
    if (f.entity_type) params.entity_type = f.entity_type
    // Backend hanya menerima RFC3339; <input type="date"> memberi "YYYY-MM-DD".
    // `to` digeser ke akhir hari supaya rentang tanggalnya inklusif -- kalau
    // dikirim apa adanya, log sore hari di tanggal akhir ikut terpotong.
    if (f.from) params.from = new Date(`${f.from}T00:00:00`).toISOString()
    if (f.to) params.to = new Date(`${f.to}T23:59:59`).toISOString()

    return apiClient
      .get('/api/audit/audit-logs', { params })
      .then(({ data }) => {
        setLogs(data)
        setError('')
      })
      .catch(() => setError('Gagal memuat audit log. Pastikan audit-service aktif.'))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    loadLogs(companyId, filters)
    // filters sengaja tidak jadi dependency: pencarian dijalankan lewat tombol,
    // bukan tiap ketikan, supaya tidak membanjiri backend.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [companyId, loadLogs])

  // Pilihan dropdown diturunkan dari data yang benar-benar ada, bukan daftar
  // hardcoded -- daftar topic bertambah tiap modul baru, dan daftar statis akan
  // langsung basi tanpa ada yang sadar.
  const eventTypes = useMemo(() => [...new Set(logs.map((l) => l.event_type))].sort(), [logs])
  const entityTypes = useMemo(() => [...new Set(logs.map((l) => l.entity_type))].sort(), [logs])

  const columns = [
    {
      key: 'occurred_at',
      label: 'Waktu',
      render: (l) => (
        <span className="text-nowrap">{new Date(l.occurred_at).toLocaleString('id-ID')}</span>
      ),
    },
    { key: 'event_type', label: 'Event', render: (l) => <code className="small">{l.event_type}</code> },
    {
      key: 'action',
      label: 'Aksi',
      render: (l) => <span className={`badge ${ACTION_BADGE[l.action] ?? 'text-bg-secondary'}`}>{l.action}</span>,
    },
    {
      key: 'entity_type',
      label: 'Entitas',
      render: (l) => (
        <div>
          <div>{l.entity_type}</div>
          {l.entity_id && <div className="text-secondary small font-monospace">{l.entity_id.slice(0, 8)}</div>}
        </div>
      ),
    },
    { key: 'source_service', label: 'Service' },
    {
      key: 'actor_email',
      label: 'Aktor',
      maxWidth: 200,
      searchValue: (l) => l.actor_email ?? l.actor_user_id ?? '',
      render: (l) => <TruncatedText value={l.actor_email ?? l.actor_user_id} maxWidth={200} />,
    },
    {
      key: 'actions',
      label: '',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (l) => (
        <button
          type="button"
          className="btn btn-sm btn-outline-secondary"
          disabled={!l.payload}
          onClick={() => setDetail(l)}
        >
          <i className="bi bi-braces" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">Audit Log</h2>
        <div className="text-secondary small">
          Jejak perubahan dari seluruh service, dikumpulkan lewat Kafka. Hanya bisa dibaca.
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <form
          className="row g-2 align-items-end mb-3"
          onSubmit={(e) => {
            e.preventDefault()
            loadLogs(companyId, filters)
          }}
        >
          <div className="col-md-3">
            <label className="form-label small">Event</label>
            <input
              type="text"
              list="audit-event-types"
              className="form-control form-control-sm"
              placeholder="mis. company.branch.updated"
              value={filters.event_type}
              onChange={(e) => setFilters({ ...filters, event_type: e.target.value })}
            />
            <datalist id="audit-event-types">
              {eventTypes.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          </div>
          <div className="col-md-2">
            <label className="form-label small">Entitas</label>
            <input
              type="text"
              list="audit-entity-types"
              className="form-control form-control-sm"
              placeholder="mis. branch"
              value={filters.entity_type}
              onChange={(e) => setFilters({ ...filters, entity_type: e.target.value })}
            />
            <datalist id="audit-entity-types">
              {entityTypes.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          </div>
          <div className="col-md-2">
            <label className="form-label small">Dari</label>
            <input
              type="date"
              className="form-control form-control-sm"
              value={filters.from}
              onChange={(e) => setFilters({ ...filters, from: e.target.value })}
            />
          </div>
          <div className="col-md-2">
            <label className="form-label small">Sampai</label>
            <input
              type="date"
              className="form-control form-control-sm"
              value={filters.to}
              onChange={(e) => setFilters({ ...filters, to: e.target.value })}
            />
          </div>
          <div className="col-md-1">
            {/* Backend membatasi limit di 500; nilai di luar itu diabaikan
                diam-diam dan kembali ke 100, jadi pilihannya dibatasi di sini. */}
            <label className="form-label small">Limit</label>
            <select
              className="form-select form-select-sm"
              value={filters.limit}
              onChange={(e) => setFilters({ ...filters, limit: e.target.value })}
            >
              <option value="50">50</option>
              <option value="100">100</option>
              <option value="250">250</option>
              <option value="500">500</option>
            </select>
          </div>
          <div className="col-md-2 d-flex gap-2">
            {can('create') && (
              <button type="submit" className="btn btn-primary btn-sm flex-grow-1" disabled={loading}>
                <i className="bi bi-funnel me-1" />
                Terapkan
              </button>
            )}
            <button
              type="button"
              className="btn btn-outline-secondary btn-sm"
              onClick={() => {
                setFilters(emptyFilters)
                loadLogs(companyId, emptyFilters)
              }}
            >
              Reset
            </button>
          </div>
        </form>

        <DataTable
          columns={columns}
          data={logs}
          loading={loading}
          pageSize={15}
          searchPlaceholder="Cari event, entitas, service, atau aktor..."
          emptyMessage="Belum ada audit log yang cocok. Log baru terisi kalau Kafka aktif saat perubahan terjadi."
        />
      </div>

      {detail && (
        <Modal
          title={detail.event_type}
          onClose={() => setDetail(null)}
          footer={
            <button type="button" className="btn btn-outline-secondary" onClick={() => setDetail(null)}>
              Tutup
            </button>
          }
        >
          <div className="d-flex flex-column gap-3">
            <dl className="row mb-0 small">
              <dt className="col-4 text-secondary">Waktu</dt>
              <dd className="col-8">{new Date(detail.occurred_at).toLocaleString('id-ID')}</dd>
              <dt className="col-4 text-secondary">Service</dt>
              <dd className="col-8">{detail.source_service}</dd>
              <dt className="col-4 text-secondary">Entitas</dt>
              <dd className="col-8 font-monospace">
                {detail.entity_type} / {detail.entity_id}
              </dd>
              <dt className="col-4 text-secondary">Aktor</dt>
              <dd className="col-8">{detail.actor_email ?? detail.actor_user_id ?? '—'}</dd>
            </dl>
            <div>
              <div className="text-secondary small mb-1">Payload</div>
              <pre className="bg-body-tertiary p-2 rounded small mb-0" style={{ maxHeight: 320, overflow: 'auto' }}>
                {JSON.stringify(detail.payload, null, 2)}
              </pre>
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

export default AuditLogPage
