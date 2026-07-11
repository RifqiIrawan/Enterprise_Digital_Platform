import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { asset_id: '', maintenance_type: '', scheduled_date: new Date().toISOString().slice(0, 10), notes: '' }

const STATUS_BADGE = {
  SCHEDULED: 'text-bg-info',
  COMPLETED: 'text-bg-success',
  CANCELLED: 'text-bg-secondary',
}

function isOverdue(schedule) {
  if (schedule.status !== 'SCHEDULED') return false
  const today = new Date().toISOString().slice(0, 10)
  return schedule.scheduled_date.slice(0, 10) < today
}

function MaintenanceSchedulePage() {
  const { companyId } = useCompany()
  const [assets, setAssets] = useState([])
  const [schedules, setSchedules] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState(null)

  function loadSchedules(cid) {
    setLoading(true)
    apiClient
      .get('/api/asset/maintenance-schedules', { params: { company_id: cid } })
      .then(({ data }) => setSchedules(data))
      .catch(() => setError('Gagal memuat data jadwal maintenance. Pastikan asset-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadSchedules(companyId)
    apiClient.get('/api/asset/assets', { params: { company_id: companyId } }).then(({ data }) => setAssets(data))
  }, [companyId])

  const assetName = (id) => {
    const a = assets.find((a) => a.id === id)
    return a ? `${a.asset_code} - ${a.name}` : id
  }

  function openCreate() {
    setForm({ ...emptyForm })
    setFormError('')
    setCreating(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/asset/maintenance-schedules', {
        company_id: companyId,
        asset_id: form.asset_id,
        maintenance_type: form.maintenance_type,
        scheduled_date: form.scheduled_date,
        notes: form.notes,
      })
      setCreating(false)
      loadSchedules(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat jadwal maintenance')
    } finally {
      setSaving(false)
    }
  }

  async function handleAction(id, action) {
    setActingId(id)
    try {
      await apiClient.post(`/api/asset/maintenance-schedules/${id}/${action}`)
      loadSchedules(companyId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal memproses jadwal maintenance')
    } finally {
      setActingId(null)
    }
  }

  const columns = [
    { key: 'asset_id', label: 'Aset', render: (m) => assetName(m.asset_id), sortValue: (m) => assetName(m.asset_id) },
    { key: 'maintenance_type', label: 'Jenis Maintenance' },
    {
      key: 'scheduled_date',
      label: 'Tanggal Rencana',
      cellClassName: 'text-secondary small',
      render: (m) => (
        <span className={isOverdue(m) ? 'text-danger fw-semibold' : ''}>
          {new Date(m.scheduled_date).toLocaleDateString('id-ID')}
          {isOverdue(m) && ' (Overdue)'}
        </span>
      ),
    },
    {
      key: 'completed_date',
      label: 'Tanggal Selesai',
      cellClassName: 'text-secondary small',
      render: (m) => (m.completed_date ? new Date(m.completed_date).toLocaleDateString('id-ID') : '—'),
    },
    {
      key: 'status',
      label: 'Status',
      render: (m) => <span className={`badge ${STATUS_BADGE[m.status] ?? 'text-bg-secondary'}`}>{m.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (m) =>
        m.status === 'SCHEDULED' && (
          <div className="d-flex gap-1 justify-content-end">
            <button type="button" className="btn btn-sm btn-outline-success" disabled={actingId === m.id} onClick={() => handleAction(m.id, 'complete')}>
              Selesai
            </button>
            <button type="button" className="btn btn-sm btn-outline-danger" disabled={actingId === m.id} onClick={() => handleAction(m.id, 'cancel')}>
              Batal
            </button>
          </div>
        ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Maintenance Schedule</h2>
          <div className="text-secondary small">Jadwal perawatan rutin/insidental per aset.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId || assets.length === 0} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Jadwalkan Maintenance
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {assets.length === 0 && !loading && !error && (
        <div className="alert alert-warning py-2 small mb-0">Belum ada aset. Tambahkan data aset dulu di menu Pendataan Aset.</div>
      )}

      <div className="card p-3">
        <DataTable columns={columns} data={schedules} loading={loading} searchPlaceholder="Cari jenis maintenance..." emptyMessage="Belum ada jadwal maintenance." />
      </div>

      {creating && (
        <Modal
          title="Jadwalkan Maintenance"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="maintenance-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="maintenance-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div>
              <label className="form-label">Aset</label>
              <select className="form-select" value={form.asset_id} onChange={(e) => setForm({ ...form, asset_id: e.target.value })} required>
                <option value="">Pilih aset...</option>
                {assets.map((a) => (
                  <option key={a.id} value={a.id}>{a.asset_code} - {a.name}</option>
                ))}
              </select>
            </div>
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Jenis Maintenance</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.maintenance_type}
                  onChange={(e) => setForm({ ...form, maintenance_type: e.target.value })}
                  placeholder="Preventive, Kalibrasi, Perbaikan, ..."
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Rencana</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.scheduled_date}
                  onChange={(e) => setForm({ ...form, scheduled_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-12">
                <label className="form-label">Catatan</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.notes}
                  onChange={(e) => setForm({ ...form, notes: e.target.value })}
                />
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default MaintenanceSchedulePage
