import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

// Alasan downtime sengaja terbatas (sama persis dengan CHECK di database)
// supaya bisa dijumlahkan lintas run jadi Pareto penyebab berhenti di halaman
// OEE -- kolom teks bebas tidak bisa dikelompokkan.
const DOWNTIME_REASONS = [
  { code: 'BREAKDOWN', label: 'Kerusakan mesin' },
  { code: 'SETUP', label: 'Setup / ganti cetakan' },
  { code: 'MATERIAL_SHORTAGE', label: 'Bahan baku habis' },
  { code: 'NO_OPERATOR', label: 'Tidak ada operator' },
  { code: 'ADJUSTMENT', label: 'Penyetelan' },
  { code: 'PLANNED_STOP', label: 'Berhenti terencana' },
  { code: 'OTHER', label: 'Lainnya' },
]

const emptyForm = { work_order_id: '', machine_id: '', shift_id: '', run_date: new Date().toISOString().slice(0, 10), notes: '' }
const emptyDowntime = { reason_code: 'BREAKDOWN', minutes: 15, notes: '' }

const reasonLabel = (code) => DOWNTIME_REASONS.find((r) => r.code === code)?.label ?? code
const percent = (v) => `${(Number(v) * 100).toFixed(1)}%`

function ProductionRunsPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [runs, setRuns] = useState([])
  const [machines, setMachines] = useState([])
  const [shifts, setShifts] = useState([])
  const [workOrders, setWorkOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const [detail, setDetail] = useState(null)
  const [detailError, setDetailError] = useState('')
  const [downtimeForm, setDowntimeForm] = useState(emptyDowntime)
  const [closeForm, setCloseForm] = useState({ quantity_good: 0, quantity_reject: 0 })
  const [busy, setBusy] = useState(false)

  function loadRuns(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/production/production-runs', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setRuns(data))
      .catch(() => setError('Gagal memuat eksekusi produksi. Pastikan production-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadRuns(companyId, branchId)
    apiClient.get('/api/production/machines', { params: { company_id: companyId, status: 'ACTIVE' } }).then(({ data }) => setMachines(data))
    apiClient.get('/api/production/shifts', { params: { company_id: companyId } }).then(({ data }) => setShifts(data.filter((s) => s.is_active)))
    apiClient
      .get('/api/production/work-orders', { params: { company_id: companyId } })
      .then(({ data }) => setWorkOrders(data.filter((wo) => wo.status === 'IN_PROGRESS')))
  }, [companyId, branchId])

  const machineName = (id) => {
    const m = machines.find((m) => m.id === id) ?? detail?.machine
    return m && m.id === id ? `${m.code} - ${m.name}` : id
  }
  const shiftName = (id) => shifts.find((s) => s.id === id)?.name ?? id
  const woNumber = (id) => workOrders.find((wo) => wo.id === id)?.wo_number ?? id

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
      await apiClient.post('/api/production/production-runs', {
        company_id: companyId,
        branch_id: branchId || null,
        work_order_id: form.work_order_id,
        machine_id: form.machine_id,
        shift_id: form.shift_id,
        run_date: form.run_date,
        notes: form.notes,
      })
      setCreating(false)
      loadRuns(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat eksekusi produksi')
    } finally {
      setSaving(false)
    }
  }

  async function openDetail(runId) {
    setDetailError('')
    setDowntimeForm(emptyDowntime)
    setCloseForm({ quantity_good: 0, quantity_reject: 0 })
    try {
      const { data } = await apiClient.get(`/api/production/production-runs/${runId}`)
      setDetail(data)
    } catch {
      setError('Gagal memuat rincian eksekusi produksi.')
    }
  }

  async function refreshDetail(runId) {
    const { data } = await apiClient.get(`/api/production/production-runs/${runId}`)
    setDetail(data)
    loadRuns(companyId, branchId)
  }

  async function handleAddDowntime(e) {
    e.preventDefault()
    setBusy(true)
    setDetailError('')
    try {
      await apiClient.post(`/api/production/production-runs/${detail.id}/downtime`, {
        reason_code: downtimeForm.reason_code,
        minutes: Number(downtimeForm.minutes) || 0,
        notes: downtimeForm.notes,
      })
      setDowntimeForm(emptyDowntime)
      await refreshDetail(detail.id)
    } catch (err) {
      setDetailError(err.response?.data?.error ?? 'Gagal mencatat downtime')
    } finally {
      setBusy(false)
    }
  }

  async function handleDeleteDowntime(logId) {
    setBusy(true)
    setDetailError('')
    try {
      await apiClient.delete(`/api/production/production-runs/${detail.id}/downtime/${logId}`)
      await refreshDetail(detail.id)
    } catch (err) {
      setDetailError(err.response?.data?.error ?? 'Gagal menghapus catatan downtime')
    } finally {
      setBusy(false)
    }
  }

  async function handleClose(e) {
    e.preventDefault()
    setBusy(true)
    setDetailError('')
    try {
      await apiClient.post(`/api/production/production-runs/${detail.id}/close`, {
        quantity_good: Number(closeForm.quantity_good) || 0,
        quantity_reject: Number(closeForm.quantity_reject) || 0,
      })
      await refreshDetail(detail.id)
    } catch (err) {
      setDetailError(err.response?.data?.error ?? 'Gagal menutup eksekusi produksi')
    } finally {
      setBusy(false)
    }
  }

  const columns = [
    { key: 'run_number', label: 'No. Run', render: (r) => <code>{r.run_number}</code> },
    { key: 'run_date', label: 'Tanggal', render: (r) => new Date(r.run_date).toLocaleDateString('id-ID'), cellClassName: 'text-secondary small' },
    { key: 'work_order_id', label: 'Work Order', render: (r) => woNumber(r.work_order_id), sortValue: (r) => woNumber(r.work_order_id) },
    { key: 'machine_id', label: 'Mesin', render: (r) => machineName(r.machine_id), sortValue: (r) => machineName(r.machine_id) },
    { key: 'shift_id', label: 'Shift', render: (r) => shiftName(r.shift_id), sortValue: (r) => shiftName(r.shift_id) },
    { key: 'planned_minutes', label: 'Waktu Rencana', className: 'text-end', cellClassName: 'text-end', render: (r) => `${r.planned_minutes} menit` },
    {
      key: 'quantity_good',
      label: 'Bagus / Reject',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (r) => (r.quantity_good == null ? '—' : `${Number(r.quantity_good)} / ${Number(r.quantity_reject)}`),
      sortValue: (r) => Number(r.quantity_good ?? 0),
    },
    {
      key: 'status',
      label: 'Status',
      render: (r) => <span className={`badge ${r.status === 'OPEN' ? 'text-bg-info' : 'text-bg-success'}`}>{r.status === 'OPEN' ? 'BERJALAN' : 'DITUTUP'}</span>,
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
          <h2 className="edp-page-title">Eksekusi Produksi</h2>
          <div className="text-secondary small">
            Catatan pelaksanaan di lantai produksi: satu mesin, satu shift, satu hari. Tidak memutasi stok &mdash; itu tetap terjadi saat Work Order diselesaikan.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId || workOrders.length === 0 || machines.length === 0 || shifts.length === 0} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Mulai Eksekusi
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {!loading && !error && workOrders.length === 0 && (
        <div className="alert alert-warning py-2 small mb-0">
          Belum ada Work Order berstatus IN_PROGRESS. Eksekusi produksi hanya bisa dicatat untuk work order yang sudah dimulai.
        </div>
      )}
      {!loading && !error && (machines.length === 0 || shifts.length === 0) && (
        <div className="alert alert-warning py-2 small mb-0">Lengkapi dulu data Mesin (status ACTIVE) dan Shift Produksi yang aktif.</div>
      )}

      <div className="card p-3">
        <DataTable columns={columns} data={runs} loading={loading} searchPlaceholder="Cari nomor run..." emptyMessage="Belum ada eksekusi produksi." />
      </div>

      {creating && (
        <Modal
          title="Mulai Eksekusi Produksi"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="run-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Mulai'}
              </button>
            </>
          }
        >
          <form id="run-form" onSubmit={handleCreate} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div>
              <label className="form-label">Work Order (IN_PROGRESS)</label>
              <select className="form-select" value={form.work_order_id} onChange={(e) => setForm({ ...form, work_order_id: e.target.value })} required>
                <option value="">Pilih work order...</option>
                {workOrders.map((wo) => (
                  <option key={wo.id} value={wo.id}>{wo.wo_number} — rencana {Number(wo.quantity_planned)}</option>
                ))}
              </select>
            </div>
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Mesin</label>
                <select className="form-select" value={form.machine_id} onChange={(e) => setForm({ ...form, machine_id: e.target.value })} required>
                  <option value="">Pilih mesin...</option>
                  {machines.map((m) => (
                    <option key={m.id} value={m.id}>{m.code} - {m.name}</option>
                  ))}
                </select>
                <div className="form-text">Mesin yang masih menjalankan run lain tidak bisa dipakai.</div>
              </div>
              <div className="col-6">
                <label className="form-label">Shift</label>
                <select className="form-select" value={form.shift_id} onChange={(e) => setForm({ ...form, shift_id: e.target.value })} required>
                  <option value="">Pilih shift...</option>
                  {shifts.map((s) => (
                    <option key={s.id} value={s.id}>{s.name} ({s.start_time}–{s.end_time}, {s.planned_minutes} menit)</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal</label>
                <input type="date" className="form-control" value={form.run_date} onChange={(e) => setForm({ ...form, run_date: e.target.value })} required />
              </div>
              <div className="col-12">
                <label className="form-label">Catatan</label>
                <input type="text" className="form-control" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
              </div>
            </div>
          </form>
        </Modal>
      )}

      {detail && (
        <Modal
          title={`Rincian ${detail.run_number}`}
          onClose={() => setDetail(null)}
          footer={
            <button type="button" className="btn btn-outline-secondary" onClick={() => setDetail(null)}>
              Tutup
            </button>
          }
        >
          <div className="d-flex flex-column gap-3">
            {detailError && <div className="alert alert-danger py-2 small mb-0">{detailError}</div>}

            <div className="row g-2 small">
              <div className="col-6">
                <div className="text-secondary">Mesin</div>
                <div>{detail.machine ? `${detail.machine.code} - ${detail.machine.name}` : '—'}</div>
              </div>
              <div className="col-6">
                <div className="text-secondary">Waktu rencana</div>
                <div>{detail.planned_minutes} menit</div>
              </div>
              <div className="col-6">
                <div className="text-secondary">Total downtime</div>
                <div>{detail.downtime_minutes} menit</div>
              </div>
              <div className="col-6">
                <div className="text-secondary">Status</div>
                <div>{detail.status === 'OPEN' ? 'BERJALAN' : 'DITUTUP'}</div>
              </div>
            </div>

            <div>
              <div className="fw-semibold small mb-1">Catatan Downtime</div>
              {detail.downtime_logs.length === 0 && <div className="text-secondary small">Belum ada downtime tercatat.</div>}
              {detail.downtime_logs.length > 0 && (
                <ul className="list-group list-group-flush">
                  {detail.downtime_logs.map((log) => (
                    <li key={log.id} className="list-group-item px-0 py-2 d-flex justify-content-between align-items-center">
                      <span className="small">
                        {reasonLabel(log.reason_code)} — {log.minutes} menit
                        {log.notes && <span className="text-secondary"> · {log.notes}</span>}
                      </span>
                      {detail.status === 'OPEN' && can('delete') && (
                        <button type="button" className="btn btn-sm btn-outline-danger" disabled={busy} onClick={() => handleDeleteDowntime(log.id)}>
                          Hapus
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </div>

            {detail.status === 'OPEN' && can('update') && (
              <form onSubmit={handleAddDowntime} className="row g-2 align-items-end">
                <div className="col-5">
                  <label className="form-label small">Alasan</label>
                  <select className="form-select form-select-sm" value={downtimeForm.reason_code} onChange={(e) => setDowntimeForm({ ...downtimeForm, reason_code: e.target.value })}>
                    {DOWNTIME_REASONS.map((r) => (
                      <option key={r.code} value={r.code}>{r.label}</option>
                    ))}
                  </select>
                </div>
                <div className="col-3">
                  <label className="form-label small">Menit</label>
                  <input
                    type="number"
                    min="1"
                    className="form-control form-control-sm"
                    value={downtimeForm.minutes}
                    onChange={(e) => setDowntimeForm({ ...downtimeForm, minutes: e.target.value })}
                    required
                  />
                </div>
                <div className="col-4">
                  <button type="submit" className="btn btn-sm btn-outline-primary w-100" disabled={busy}>
                    Catat Downtime
                  </button>
                </div>
              </form>
            )}

            {detail.status === 'OPEN' && can('update') && (
              <form onSubmit={handleClose} className="border-top pt-3 d-flex flex-column gap-2">
                <div className="fw-semibold small">Tutup Run</div>
                <div className="text-secondary small">
                  Setelah ditutup, angka hasil tidak bisa diubah dan OEE shift ini dihitung. Unit reject tidak ikut masuk gudang saat Work Order diselesaikan.
                </div>
                <div className="row g-2">
                  <div className="col-6">
                    <label className="form-label small">Unit Bagus</label>
                    <input
                      type="number"
                      min="0"
                      className="form-control form-control-sm"
                      value={closeForm.quantity_good}
                      onChange={(e) => setCloseForm({ ...closeForm, quantity_good: e.target.value })}
                      required
                    />
                  </div>
                  <div className="col-6">
                    <label className="form-label small">Unit Reject</label>
                    <input
                      type="number"
                      min="0"
                      className="form-control form-control-sm"
                      value={closeForm.quantity_reject}
                      onChange={(e) => setCloseForm({ ...closeForm, quantity_reject: e.target.value })}
                      required
                    />
                  </div>
                </div>
                <button type="submit" className="btn btn-sm btn-primary align-self-start" disabled={busy}>
                  {busy ? 'Memproses...' : 'Tutup Run'}
                </button>
              </form>
            )}

            {detail.oee && (
              <div className="border-top pt-3">
                <div className="fw-semibold small mb-2">OEE Run Ini</div>
                <div className="row g-2 small">
                  <div className="col-3">
                    <div className="text-secondary">Availability</div>
                    <div className="fs-6">{percent(detail.oee.availability)}</div>
                  </div>
                  <div className="col-3">
                    <div className="text-secondary">Performance</div>
                    <div className="fs-6">{percent(detail.oee.performance)}</div>
                  </div>
                  <div className="col-3">
                    <div className="text-secondary">Quality</div>
                    <div className="fs-6">{percent(detail.oee.quality)}</div>
                  </div>
                  <div className="col-3">
                    <div className="text-secondary">OEE</div>
                    <div className="fs-6 fw-semibold">{percent(detail.oee.oee)}</div>
                  </div>
                </div>
                <div className="text-secondary small mt-2">
                  Waktu jalan {detail.oee.run_time_minutes} dari {detail.oee.planned_minutes} menit; {Number(detail.oee.quantity_good)} bagus dari{' '}
                  {Number(detail.oee.quantity_good) + Number(detail.oee.quantity_reject)} unit.
                </div>
                {detail.oee.performance_capped && (
                  <div className="alert alert-warning py-2 small mt-2 mb-0">
                    Performance melebihi 100% dan dipotong ke 100%. Cycle time ideal mesin ini kemungkinan disetel terlalu lambat, atau ada downtime yang belum dicatat.
                  </div>
                )}
              </div>
            )}
          </div>
        </Modal>
      )}
    </div>
  )
}

export default ProductionRunsPage
