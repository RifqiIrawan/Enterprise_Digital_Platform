import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  APPROVED: 'text-bg-success',
  REJECTED: 'text-bg-danger',
}

const currency = (v) =>
  new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(v ?? 0)

function currentPeriod() {
  return new Date().toISOString().slice(0, 7)
}

function emptyForm() {
  return {
    employee_id: '',
    work_date: new Date().toISOString().slice(0, 10),
    hours: '',
    is_holiday: false,
    hourly_rate: '',
    description: '',
  }
}

function OvertimePage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [employees, setEmployees] = useState([])
  const [logs, setLogs] = useState([])
  const [period, setPeriod] = useState(currentPeriod())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionError, setActionError] = useState('')
  const [busyId, setBusyId] = useState(null)

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm())
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const [rejecting, setRejecting] = useState(null)
  const [rejectionReason, setRejectionReason] = useState('')

  function loadLogs(cid, p, bid) {
    setLoading(true)
    apiClient
      .get('/api/hr/overtime', { params: { company_id: cid, period: p, branch_id: bid } })
      .then(({ data }) => setLogs(data))
      .catch(() => setError('Gagal memuat data lembur. Pastikan hr-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    apiClient
      .get('/api/hr/employees', { params: { company_id: companyId, status: 'ACTIVE' } })
      .then(({ data }) => setEmployees(data))
      .catch(() => setEmployees([]))
  }, [companyId])

  useEffect(() => {
    if (!companyId) return
    loadLogs(companyId, period, branchId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [companyId, period, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm())
    setFormError('')
    setEditing(true)
  }

  function openEdit(log) {
    setEditingId(log.id)
    setForm({
      employee_id: log.employee_id,
      work_date: log.work_date.slice(0, 10),
      hours: String(log.hours),
      is_holiday: log.is_holiday,
      hourly_rate: String(log.hourly_rate),
      description: log.description ?? '',
    })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const payload = {
        company_id: companyId,
        branch_id: branchId || null,
        employee_id: form.employee_id,
        work_date: form.work_date,
        hours: Number(form.hours) || 0,
        is_holiday: form.is_holiday,
        // Tarif dikirim HANYA kalau diisi -- kalau kosong, backend
        // menurunkannya dari gaji pokok karyawan (basic_salary / 173).
        ...(form.hourly_rate === '' ? {} : { hourly_rate: Number(form.hourly_rate) }),
        description: form.description,
      }
      if (editingId) {
        await apiClient.put(`/api/hr/overtime/${editingId}`, payload)
      } else {
        await apiClient.post('/api/hr/overtime', payload)
      }
      setEditing(false)
      loadLogs(companyId, period, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan catatan lembur')
    } finally {
      setSaving(false)
    }
  }

  async function runAction(log, action, body) {
    setActionError('')
    setBusyId(log.id)
    try {
      await apiClient.post(`/api/hr/overtime/${log.id}/${action}`, body ?? {})
      loadLogs(companyId, period, branchId)
      return true
    } catch (err) {
      setActionError(err.response?.data?.error ?? `Gagal menjalankan aksi ${action}`)
      return false
    } finally {
      setBusyId(null)
    }
  }

  async function confirmReject(e) {
    e.preventDefault()
    const ok = await runAction(rejecting, 'reject', { rejection_reason: rejectionReason })
    if (ok) setRejecting(null)
  }

  const employeeName = (log) => {
    const emp = employees.find((e) => e.id === log.employee_id)
    return emp ? `${emp.first_name} ${emp.last_name ?? ''}`.trim() : log.employee_name
  }

  const approvedTotal = logs.filter((l) => l.status === 'APPROVED').reduce((sum, l) => sum + (l.amount ?? 0), 0)

  const columns = [
    {
      key: 'work_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (l) => (
        <div>
          <div>{new Date(l.work_date).toLocaleDateString('id-ID')}</div>
          {l.is_holiday && <span className="badge text-bg-info">Hari Libur</span>}
        </div>
      ),
    },
    {
      key: 'employee_name',
      label: 'Karyawan',
      render: (l) => employeeName(l),
      sortValue: (l) => employeeName(l),
    },
    {
      key: 'hours',
      label: 'Jam',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (l) => new Intl.NumberFormat('id-ID').format(l.hours),
    },
    {
      key: 'hourly_rate',
      label: 'Tarif/Jam',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (l) => currency(l.hourly_rate),
    },
    {
      key: 'amount',
      label: 'Upah Lembur',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (l) => currency(l.amount),
    },
    {
      key: 'status',
      label: 'Status',
      render: (l) => (
        <div>
          <span className={`badge ${STATUS_BADGE[l.status] ?? 'text-bg-secondary'}`}>{l.status}</span>
          {l.payroll_run_id && <div className="text-secondary small">Sudah masuk payroll</div>}
          {l.status === 'REJECTED' && l.rejection_reason && (
            <div className="text-secondary small">{l.rejection_reason}</div>
          )}
        </div>
      ),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      // Lembur yang sudah ikut payroll run tidak punya aksi apa pun: nilainya
      // sudah terhitung di gross, koreksinya lewat modul Finance.
      render: (l) => (
        <div className="d-flex gap-1 justify-content-end">
          {l.status === 'DRAFT' && !l.payroll_run_id && (
            <>
              {can('update') && (
                <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(l)}>
                  <i className="bi bi-pencil" />
                </button>
              )}
              {can('approve') && (
                <button
                  type="button"
                  className="btn btn-sm btn-outline-success"
                  disabled={busyId === l.id}
                  onClick={() => runAction(l, 'approve')}
                >
                  <i className="bi bi-check-lg me-1" />
                  Setujui
                </button>
              )}
            </>
          )}
          {(l.status === 'DRAFT' || l.status === 'APPROVED') && !l.payroll_run_id && can('approve') && (
            <button
              type="button"
              className="btn btn-sm btn-outline-danger"
              disabled={busyId === l.id}
              onClick={() => {
                setRejectionReason('')
                setRejecting(l)
              }}
            >
              <i className="bi bi-x-lg me-1" />
              Tolak
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
          <h2 className="edp-page-title">Lembur</h2>
          <div className="text-secondary small">
            Pengali upah mengikuti Kepmenaker 102/2004: hari kerja 1,5x jam pertama lalu 2x; hari libur 2x/3x/4x.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Catat Lembur
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {actionError && <div className="alert alert-warning py-2 small">{actionError}</div>}

      <div className="card p-3">
        <div className="d-flex align-items-center gap-2 mb-2">
          <label className="form-label mb-0 small text-secondary">Periode</label>
          <input
            type="month"
            className="form-control form-control-sm"
            style={{ maxWidth: 160 }}
            value={period}
            onChange={(e) => setPeriod(e.target.value)}
          />
          <span className="ms-auto small text-secondary">
            Disetujui periode ini: <strong className="text-body">{currency(approvedTotal)}</strong>
          </span>
        </div>
        <DataTable
          columns={columns}
          data={logs}
          loading={loading}
          searchPlaceholder="Cari karyawan..."
          emptyMessage="Belum ada catatan lembur di periode ini."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Lembur' : 'Catat Lembur'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="overtime-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="overtime-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-12">
                <label className="form-label">Karyawan</label>
                <select
                  className="form-select"
                  value={form.employee_id}
                  onChange={(e) => setForm({ ...form, employee_id: e.target.value })}
                  disabled={!!editingId}
                  required
                >
                  <option value="">Pilih karyawan...</option>
                  {employees.map((emp) => (
                    <option key={emp.id} value={emp.id}>
                      {emp.employee_code} - {emp.first_name} {emp.last_name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.work_date}
                  onChange={(e) => setForm({ ...form, work_date: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Jam Lembur</label>
                <input
                  type="number"
                  step="0.5"
                  min="0.5"
                  max="12"
                  className="form-control"
                  value={form.hours}
                  onChange={(e) => setForm({ ...form, hours: e.target.value })}
                  required
                />
              </div>
              <div className="col-12">
                <div className="form-check">
                  <input
                    id="overtime-is-holiday"
                    type="checkbox"
                    className="form-check-input"
                    checked={form.is_holiday}
                    onChange={(e) => setForm({ ...form, is_holiday: e.target.checked })}
                  />
                  <label className="form-check-label" htmlFor="overtime-is-holiday">
                    Hari libur / istirahat mingguan
                  </label>
                </div>
              </div>
              <div className="col-12">
                <label className="form-label">Tarif per Jam</label>
                <input
                  type="number"
                  min="0"
                  className="form-control"
                  placeholder="Kosongkan untuk memakai gaji pokok / 173"
                  value={form.hourly_rate}
                  onChange={(e) => setForm({ ...form, hourly_rate: e.target.value })}
                />
              </div>
              <div className="col-12">
                <label className="form-label">Keterangan</label>
                <textarea
                  className="form-control"
                  rows={2}
                  value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                />
              </div>
            </div>
          </form>
        </Modal>
      )}

      {rejecting && (
        <Modal
          title="Tolak Lembur"
          onClose={() => setRejecting(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setRejecting(null)}>
                Batal
              </button>
              <button
                type="submit"
                form="overtime-reject-form"
                className="btn btn-danger"
                disabled={busyId === rejecting.id}
              >
                Tolak
              </button>
            </>
          }
        >
          <form id="overtime-reject-form" onSubmit={confirmReject} className="d-flex flex-column gap-2">
            <div className="text-secondary small">
              {employeeName(rejecting)} &middot; {new Date(rejecting.work_date).toLocaleDateString('id-ID')} &middot;{' '}
              {rejecting.hours} jam
            </div>
            <label className="form-label mb-0">Alasan penolakan</label>
            <textarea
              className="form-control"
              rows={3}
              value={rejectionReason}
              onChange={(e) => setRejectionReason(e.target.value)}
              required
            />
          </form>
        </Modal>
      )}
    </div>
  )
}

export default OvertimePage
