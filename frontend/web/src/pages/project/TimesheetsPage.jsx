import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = {
  project_id: '',
  task_id: '',
  employee_id: '',
  work_date: '',
  hours: '',
  hourly_rate: '',
  description: '',
}

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  APPROVED: 'text-bg-primary',
  POSTED: 'text-bg-success',
  REJECTED: 'text-bg-danger',
}

// Timesheet hanya bisa dicatat pada proyek ACTIVE (backend menolak yang lain).
const LOGGABLE_PROJECT_STATUS = 'ACTIVE'

const currency = (v) => new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(v ?? 0)

function TimesheetsPage() {
  const { companyId, branchId } = useCompany()
  const [timesheets, setTimesheets] = useState([])
  const [projects, setProjects] = useState([])
  const [tasks, setTasks] = useState([])
  const [employees, setEmployees] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionError, setActionError] = useState('')
  const [busyId, setBusyId] = useState(null)

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadTimesheets(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/project/timesheets', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setTimesheets(data))
      .catch(() => setError('Gagal memuat data timesheet. Pastikan project-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadTimesheets(companyId, branchId)
    apiClient
      .get('/api/project/projects', { params: { company_id: companyId, branch_id: branchId } })
      .then(({ data }) => setProjects(data))
      .catch(() => setProjects([]))
    apiClient
      .get('/api/project/tasks', { params: { company_id: companyId, branch_id: branchId } })
      .then(({ data }) => setTasks(data))
      .catch(() => setTasks([]))
    apiClient
      .get('/api/hr/employees', { params: { company_id: companyId, status: 'ACTIVE' } })
      .then(({ data }) => setEmployees(data))
      .catch(() => setEmployees([]))
  }, [companyId, branchId])

  function openCreate() {
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/project/timesheets', {
        company_id: companyId,
        branch_id: branchId || null,
        project_id: form.project_id,
        task_id: form.task_id,
        employee_id: form.employee_id,
        work_date: form.work_date,
        hours: Number(form.hours) || 0,
        // Tarif dikirim HANYA kalau diisi -- kalau kosong, backend menurunkannya
        // dari gaji pokok karyawan di hr-service (basic_salary / 173).
        ...(form.hourly_rate === '' ? {} : { hourly_rate: Number(form.hourly_rate) }),
        description: form.description,
      })
      setEditing(false)
      loadTimesheets(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal mencatat timesheet')
    } finally {
      setSaving(false)
    }
  }

  async function runAction(timesheet, action) {
    setActionError('')
    setBusyId(timesheet.id)
    try {
      await apiClient.post(`/api/project/timesheets/${timesheet.id}/${action}`)
      loadTimesheets(companyId, branchId)
    } catch (err) {
      setActionError(err.response?.data?.error ?? `Gagal menjalankan aksi ${action}`)
    } finally {
      setBusyId(null)
    }
  }

  const projectLabel = (id) => {
    const p = projects.find((x) => x.id === id)
    return p ? `${p.project_code} — ${p.name}` : '-'
  }

  const taskLabel = (id) => {
    if (!id) return null
    const t = tasks.find((x) => x.id === id)
    return t ? `${t.task_number} — ${t.title}` : null
  }

  const columns = [
    {
      key: 'work_date',
      label: 'Tanggal',
      render: (t) => new Date(t.work_date).toLocaleDateString('id-ID'),
    },
    {
      key: 'employee_name',
      label: 'Karyawan',
      render: (t) => (
        <div>
          <div>{t.employee_name}</div>
          <div className="text-secondary small">{projectLabel(t.project_id)}</div>
        </div>
      ),
    },
    {
      key: 'task_id',
      label: 'Tugas',
      render: (t) => taskLabel(t.task_id) ?? <span className="text-secondary">(umum)</span>,
    },
    {
      key: 'hours',
      label: 'Jam',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (t) => new Intl.NumberFormat('id-ID').format(t.hours),
    },
    {
      key: 'hourly_rate',
      label: 'Tarif/Jam',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (t) => currency(t.hourly_rate),
    },
    {
      key: 'amount',
      label: 'Jumlah',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (t) => currency(t.amount),
    },
    {
      key: 'status',
      label: 'Status',
      render: (t) => <span className={`badge ${STATUS_BADGE[t.status] ?? 'text-bg-secondary'}`}>{t.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      // POSTED tidak punya aksi apa pun di sini: biayanya sudah masuk jurnal
      // finance-service, koreksinya lewat jurnal balik di modul Finance.
      render: (t) => (
        <div className="d-flex gap-1 justify-content-end">
          {t.status === 'DRAFT' && (
            <button
              type="button"
              className="btn btn-sm btn-outline-primary"
              disabled={busyId === t.id}
              onClick={() => runAction(t, 'approve')}
            >
              <i className="bi bi-check-lg me-1" />
              Setujui
            </button>
          )}
          {(t.status === 'DRAFT' || t.status === 'APPROVED') && (
            <button
              type="button"
              className="btn btn-sm btn-outline-danger"
              disabled={busyId === t.id}
              onClick={() => runAction(t, 'reject')}
            >
              <i className="bi bi-x-lg me-1" />
              Tolak
            </button>
          )}
        </div>
      ),
    },
  ]

  const activeProjects = projects.filter((p) => p.status === LOGGABLE_PROJECT_STATUS)
  const tasksOfSelectedProject = tasks.filter((t) => t.project_id === form.project_id)

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Timesheet</h2>
          <div className="text-secondary small">
            Timesheet APPROVED diposting ke GL secara kolektif per proyek dari menu Proyek.
          </div>
        </div>
        <button
          type="button"
          className="btn btn-primary btn-sm"
          disabled={!companyId || activeProjects.length === 0}
          onClick={openCreate}
        >
          <i className="bi bi-plus-lg me-1" />
          Catat Jam Kerja
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {actionError && <div className="alert alert-warning py-2 small">{actionError}</div>}
      {companyId && activeProjects.length === 0 && !loading && (
        <div className="alert alert-info py-2 small">
          Belum ada proyek berstatus ACTIVE. Aktifkan proyek dulu di menu Proyek sebelum mencatat jam kerja.
        </div>
      )}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={timesheets}
          loading={loading}
          searchPlaceholder="Cari nama karyawan..."
          emptyMessage="Belum ada timesheet."
        />
      </div>

      {editing && (
        <Modal
          title="Catat Jam Kerja"
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="timesheet-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="timesheet-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-12">
                <label className="form-label">Proyek</label>
                <select
                  className="form-select"
                  value={form.project_id}
                  onChange={(e) => setForm({ ...form, project_id: e.target.value, task_id: '' })}
                  required
                >
                  <option value="">Pilih proyek...</option>
                  {activeProjects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.project_code} — {p.name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-12">
                <label className="form-label">Tugas (opsional)</label>
                <select
                  className="form-select"
                  value={form.task_id}
                  onChange={(e) => setForm({ ...form, task_id: e.target.value })}
                  disabled={!form.project_id}
                >
                  <option value="">(jam kerja umum proyek)</option>
                  {tasksOfSelectedProject.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.task_number} — {t.title}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-12">
                <label className="form-label">Karyawan</label>
                <select
                  className="form-select"
                  value={form.employee_id}
                  onChange={(e) => setForm({ ...form, employee_id: e.target.value })}
                  required
                >
                  <option value="">Pilih karyawan...</option>
                  {employees.map((emp) => (
                    <option key={emp.id} value={emp.id}>
                      {emp.employee_code} — {emp.first_name} {emp.last_name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-4">
                <label className="form-label">Tanggal</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.work_date}
                  onChange={(e) => setForm({ ...form, work_date: e.target.value })}
                />
              </div>
              <div className="col-4">
                <label className="form-label">Jam</label>
                <input
                  type="number"
                  min="0.5"
                  max="24"
                  step="0.5"
                  className="form-control"
                  value={form.hours}
                  onChange={(e) => setForm({ ...form, hours: e.target.value })}
                  required
                />
              </div>
              <div className="col-4">
                <label className="form-label">Tarif/Jam</label>
                <input
                  type="number"
                  min="0"
                  step="0.01"
                  className="form-control"
                  placeholder="Dari gaji pokok"
                  value={form.hourly_rate}
                  onChange={(e) => setForm({ ...form, hourly_rate: e.target.value })}
                />
                <div className="form-text">Kosongkan untuk memakai gaji pokok karyawan dibagi 173.</div>
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
    </div>
  )
}

export default TimesheetsPage
