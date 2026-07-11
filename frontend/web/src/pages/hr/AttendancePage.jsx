import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const STATUSES = ['PRESENT', 'LATE', 'EARLY_LEAVE', 'ABSENT', 'LEAVE']

const STATUS_BADGE = {
  PRESENT: 'text-bg-success',
  LATE: 'text-bg-warning',
  EARLY_LEAVE: 'text-bg-warning',
  ABSENT: 'text-bg-danger',
  LEAVE: 'text-bg-info',
}

function currentPeriod() {
  return new Date().toISOString().slice(0, 7)
}

function emptyForm() {
  return {
    employee_id: '',
    log_date: new Date().toISOString().slice(0, 10),
    check_in: '',
    check_out: '',
    status: 'PRESENT',
  }
}

function toLocalTimeValue(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function combineDateTime(logDate, timeStr) {
  if (!timeStr) return null
  return new Date(`${logDate}T${timeStr}:00`).toISOString()
}

function AttendancePage() {
  const { companyId } = useCompany()
  const [employees, setEmployees] = useState([])
  const [logs, setLogs] = useState([])
  const [period, setPeriod] = useState(currentPeriod())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm())
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadLogs(cid, p) {
    setLoading(true)
    apiClient
      .get('/api/hr/attendance', { params: { company_id: cid, period: p } })
      .then(({ data }) => setLogs(data))
      .catch(() => setError('Gagal memuat data absensi. Pastikan hr-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    apiClient.get('/api/hr/employees', { params: { company_id: companyId, status: 'ACTIVE' } }).then(({ data }) => setEmployees(data))
  }, [companyId])

  useEffect(() => {
    if (!companyId) return
    loadLogs(companyId, period)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [companyId, period])

  const employeeName = (id) => {
    const emp = employees.find((e) => e.id === id)
    return emp ? `${emp.first_name} ${emp.last_name ?? ''}`.trim() : id
  }

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
      log_date: log.log_date.slice(0, 10),
      check_in: toLocalTimeValue(log.check_in),
      check_out: toLocalTimeValue(log.check_out),
      status: log.status,
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
        employee_id: form.employee_id,
        log_date: form.log_date,
        check_in: combineDateTime(form.log_date, form.check_in),
        check_out: combineDateTime(form.log_date, form.check_out),
        status: form.status,
      }
      if (editingId) {
        await apiClient.put(`/api/hr/attendance/${editingId}`, payload)
      } else {
        await apiClient.post('/api/hr/attendance', payload)
      }
      setEditing(false)
      loadLogs(companyId, period)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan absensi')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    {
      key: 'employee_id',
      label: 'Karyawan',
      render: (l) => employeeName(l.employee_id),
      sortValue: (l) => employeeName(l.employee_id),
    },
    {
      key: 'log_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (l) => new Date(l.log_date).toLocaleDateString('id-ID'),
    },
    { key: 'check_in', label: 'Jam Masuk', cellClassName: 'text-secondary small', render: (l) => toLocalTimeValue(l.check_in) || '-' },
    { key: 'check_out', label: 'Jam Pulang', cellClassName: 'text-secondary small', render: (l) => toLocalTimeValue(l.check_out) || '-' },
    {
      key: 'status',
      label: 'Status',
      render: (l) => <span className={`badge ${STATUS_BADGE[l.status] ?? 'text-bg-secondary'}`}>{l.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (l) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(l)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Absensi</h2>
          <div className="text-secondary small">Catatan kehadiran harian karyawan, dipakai sebagai dasar pro-rata payroll.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Catat Absensi
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

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
        </div>
        <DataTable
          columns={columns}
          data={logs}
          loading={loading}
          searchPlaceholder="Cari karyawan..."
          emptyMessage="Belum ada catatan absensi di periode ini."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Absensi' : 'Catat Absensi'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="attendance-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="attendance-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
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
                  value={form.log_date}
                  onChange={(e) => setForm({ ...form, log_date: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Status</label>
                <select
                  className="form-select"
                  value={form.status}
                  onChange={(e) => setForm({ ...form, status: e.target.value })}
                >
                  {STATUSES.map((s) => (
                    <option key={s} value={s}>{s}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Jam Masuk</label>
                <input
                  type="time"
                  className="form-control"
                  value={form.check_in}
                  onChange={(e) => setForm({ ...form, check_in: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Jam Pulang</label>
                <input
                  type="time"
                  className="form-control"
                  value={form.check_out}
                  onChange={(e) => setForm({ ...form, check_out: e.target.value })}
                />
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default AttendancePage
