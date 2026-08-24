import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { asset_id: '', scheduled_date: new Date().toISOString().slice(0, 10), interval_months: 12, notes: '' }
const emptyResultForm = { performed_date: new Date().toISOString().slice(0, 10), result: 'PASS', certificate_number: '', performed_by: '', notes: '' }

const STATUS_BADGE = {
  SCHEDULED: 'text-bg-info',
  COMPLETED: 'text-bg-success',
  CANCELLED: 'text-bg-secondary',
}

const RESULT_BADGE = {
  PASS: 'text-bg-success',
  ADJUSTED: 'text-bg-warning',
  FAIL: 'text-bg-danger',
}

const RESULT_LABEL = { PASS: 'Lolos', ADJUSTED: 'Disetel Ulang', FAIL: 'Gagal' }

const formatDate = (value) => (value ? new Date(value).toLocaleDateString('id-ID') : '—')

function CalibrationPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [calibrations, setCalibrations] = useState([])
  const [assets, setAssets] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const [completing, setCompleting] = useState(null)
  const [resultForm, setResultForm] = useState(emptyResultForm)
  const [resultError, setResultError] = useState('')
  const [actingId, setActingId] = useState(null)

  function loadCalibrations(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/asset/calibrations', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setCalibrations(data))
      .catch(() => setError('Gagal memuat data kalibrasi. Pastikan asset-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadCalibrations(companyId, branchId)
    apiClient
      .get('/api/asset/assets', { params: { company_id: companyId } })
      .then(({ data }) => setAssets(data.filter((a) => a.status !== 'DISPOSED')))
  }, [companyId, branchId])

  const assetName = (id) => {
    const a = assets.find((a) => a.id === id)
    return a ? `${a.asset_code} - ${a.name}` : id
  }

  // Overdue dihitung saat ditampilkan, bukan status tersendiri di database --
  // sama seperti jadwal maintenance.
  const isOverdue = (c) => c.status === 'SCHEDULED' && new Date(c.scheduled_date) < new Date(new Date().toDateString())

  function openCreate() {
    setForm({ ...emptyForm })
    setFormError('')
    setCreating(true)
  }

  async function handleCreate(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/asset/calibrations', {
        company_id: companyId,
        branch_id: branchId || null,
        asset_id: form.asset_id,
        scheduled_date: form.scheduled_date,
        interval_months: form.interval_months === '' ? null : Number(form.interval_months),
        notes: form.notes,
      })
      setCreating(false)
      loadCalibrations(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menjadwalkan kalibrasi')
    } finally {
      setSaving(false)
    }
  }

  function openComplete(c) {
    setCompleting(c)
    setResultForm({ ...emptyResultForm })
    setResultError('')
  }

  async function handleComplete(e) {
    e.preventDefault()
    setSaving(true)
    setResultError('')
    try {
      const { data } = await apiClient.post(`/api/asset/calibrations/${completing.id}/complete`, resultForm)
      setCompleting(null)
      loadCalibrations(companyId, branchId)
      if (data.result === 'FAIL') {
        window.alert('Hasil FAIL: status aset diubah menjadi MAINTENANCE dan kalibrasi berikutnya TIDAK dijadwalkan otomatis.')
      }
    } catch (err) {
      setResultError(err.response?.data?.error ?? 'Gagal menyimpan hasil kalibrasi')
    } finally {
      setSaving(false)
    }
  }

  async function handleCancel(id) {
    setActingId(id)
    try {
      await apiClient.post(`/api/asset/calibrations/${id}/cancel`)
      loadCalibrations(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal membatalkan jadwal kalibrasi')
    } finally {
      setActingId(null)
    }
  }

  const columns = [
    { key: 'asset_id', label: 'Aset', render: (c) => assetName(c.asset_id), sortValue: (c) => assetName(c.asset_id) },
    {
      key: 'scheduled_date',
      label: 'Jadwal',
      render: (c) => (
        <div className="d-flex align-items-center gap-2">
          <span>{formatDate(c.scheduled_date)}</span>
          {isOverdue(c) && <span className="badge text-bg-danger">Terlambat</span>}
        </div>
      ),
      sortValue: (c) => c.scheduled_date,
    },
    {
      key: 'interval_months',
      label: 'Interval',
      render: (c) => (c.interval_months ? `${c.interval_months} bulan` : 'sekali'),
      sortValue: (c) => c.interval_months ?? 0,
    },
    { key: 'performed_date', label: 'Dikerjakan', render: (c) => formatDate(c.performed_date), cellClassName: 'text-secondary small' },
    {
      key: 'result',
      label: 'Hasil',
      render: (c) => (c.result ? <span className={`badge ${RESULT_BADGE[c.result]}`}>{RESULT_LABEL[c.result] ?? c.result}</span> : '—'),
    },
    { key: 'next_due_date', label: 'Jatuh Tempo Berikutnya', render: (c) => formatDate(c.next_due_date), cellClassName: 'text-secondary small' },
    {
      key: 'status',
      label: 'Status',
      render: (c) => <span className={`badge ${STATUS_BADGE[c.status] ?? 'text-bg-secondary'}`}>{c.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (c) =>
        c.status === 'SCHEDULED' && (
          <div className="d-flex gap-1 justify-content-end">
            {can('update') && (
              <button type="button" className="btn btn-sm btn-outline-success" onClick={() => openComplete(c)}>
                Catat Hasil
              </button>
            )}
            {can('update') && (
              <button type="button" className="btn btn-sm btn-outline-danger" disabled={actingId === c.id} onClick={() => handleCancel(c.id)}>
                Batal
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
          <h2 className="edp-page-title">Kalibrasi</h2>
          <div className="text-secondary small">
            Kalibrasi alat ukur beserta sertifikatnya. Hasil lolos otomatis menjadwalkan kalibrasi berikutnya sesuai interval.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId || assets.length === 0} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Jadwalkan Kalibrasi
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {!loading && !error && assets.length === 0 && (
        <div className="alert alert-warning py-2 small mb-0">Belum ada aset aktif. Tambahkan aset dulu di menu Pendataan Aset.</div>
      )}

      <div className="card p-3">
        <DataTable columns={columns} data={calibrations} loading={loading} searchPlaceholder="Cari aset..." emptyMessage="Belum ada jadwal kalibrasi." />
      </div>

      {creating && (
        <Modal
          title="Jadwalkan Kalibrasi"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="calibration-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="calibration-form" onSubmit={handleCreate} className="d-flex flex-column gap-3">
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
                <label className="form-label">Tanggal Jadwal</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.scheduled_date}
                  onChange={(e) => setForm({ ...form, scheduled_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Interval (bulan)</label>
                <input
                  type="number"
                  className="form-control"
                  value={form.interval_months}
                  onChange={(e) => setForm({ ...form, interval_months: e.target.value })}
                  min="1"
                />
                <div className="form-text">Kosongkan untuk kalibrasi sekali saja.</div>
              </div>
              <div className="col-12">
                <label className="form-label">Catatan</label>
                <input type="text" className="form-control" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
              </div>
            </div>
          </form>
        </Modal>
      )}

      {completing && (
        <Modal
          title={`Catat Hasil Kalibrasi — ${assetName(completing.asset_id)}`}
          onClose={() => setCompleting(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCompleting(null)}>
                Batal
              </button>
              <button type="submit" form="calibration-result-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan Hasil'}
              </button>
            </>
          }
        >
          <form id="calibration-result-form" onSubmit={handleComplete} className="d-flex flex-column gap-3">
            {resultError && <div className="alert alert-danger py-2 small mb-0">{resultError}</div>}
            <div className="text-secondary small">
              {completing.interval_months
                ? `Hasil Lolos/Disetel Ulang akan langsung menjadwalkan kalibrasi berikutnya ${completing.interval_months} bulan setelah tanggal pengerjaan.`
                : 'Kalibrasi ini tanpa interval, jadi tidak ada jadwal berikutnya yang dibuat otomatis.'}
              {' '}Hasil Gagal menarik aset ke status MAINTENANCE dan sengaja tidak menjadwalkan ulang.
            </div>
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Tanggal Pengerjaan</label>
                <input
                  type="date"
                  className="form-control"
                  value={resultForm.performed_date}
                  onChange={(e) => setResultForm({ ...resultForm, performed_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Hasil</label>
                <select className="form-select" value={resultForm.result} onChange={(e) => setResultForm({ ...resultForm, result: e.target.value })}>
                  <option value="PASS">Lolos</option>
                  <option value="ADJUSTED">Disetel Ulang</option>
                  <option value="FAIL">Gagal</option>
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Nomor Sertifikat</label>
                <input
                  type="text"
                  className="form-control"
                  value={resultForm.certificate_number}
                  onChange={(e) => setResultForm({ ...resultForm, certificate_number: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Dikerjakan Oleh</label>
                <input
                  type="text"
                  className="form-control"
                  value={resultForm.performed_by}
                  onChange={(e) => setResultForm({ ...resultForm, performed_by: e.target.value })}
                  placeholder="Nama lembaga kalibrasi"
                />
              </div>
              <div className="col-12">
                <label className="form-label">Catatan</label>
                <input
                  type="text"
                  className="form-control"
                  value={resultForm.notes}
                  onChange={(e) => setResultForm({ ...resultForm, notes: e.target.value })}
                />
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default CalibrationPage
