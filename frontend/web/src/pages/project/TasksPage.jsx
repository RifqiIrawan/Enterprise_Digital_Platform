import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = {
  project_id: '',
  title: '',
  description: '',
  assignee_employee_id: '',
  status: 'TODO',
  priority: 'MEDIUM',
  due_date: '',
  estimated_hours: '',
}

const STATUS_BADGE = {
  TODO: 'text-bg-secondary',
  IN_PROGRESS: 'text-bg-primary',
  DONE: 'text-bg-success',
  CANCELLED: 'text-bg-danger',
}

const PRIORITY_BADGE = {
  LOW: 'text-bg-light text-dark',
  MEDIUM: 'text-bg-info',
  HIGH: 'text-bg-danger',
}

const STATUSES = ['TODO', 'IN_PROGRESS', 'DONE', 'CANCELLED']
const PRIORITIES = ['LOW', 'MEDIUM', 'HIGH']

// Proyek yang sudah ditutup tidak menerima tugas baru (backend menolak), jadi
// dropdown proyek hanya menawarkan yang masih terbuka.
const OPEN_PROJECT_STATUSES = ['PLANNING', 'ACTIVE', 'ON_HOLD']

function TasksPage() {
  const { companyId, branchId } = useCompany()
  const [tasks, setTasks] = useState([])
  const [projects, setProjects] = useState([])
  const [employees, setEmployees] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadTasks(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/project/tasks', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setTasks(data))
      .catch(() => setError('Gagal memuat data tugas. Pastikan project-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadTasks(companyId, branchId)
    apiClient
      .get('/api/project/projects', { params: { company_id: companyId, branch_id: branchId } })
      .then(({ data }) => setProjects(data))
      .catch(() => setProjects([]))
    apiClient
      .get('/api/hr/employees', { params: { company_id: companyId, status: 'ACTIVE' } })
      .then(({ data }) => setEmployees(data))
      .catch(() => setEmployees([]))
  }, [companyId, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(t) {
    setEditingId(t.id)
    setForm({
      project_id: t.project_id,
      title: t.title,
      description: t.description ?? '',
      assignee_employee_id: t.assignee_employee_id ?? '',
      status: t.status,
      priority: t.priority,
      due_date: (t.due_date ?? '').slice(0, 10),
      estimated_hours: t.estimated_hours ?? '',
    })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    const payload = {
      company_id: companyId,
      title: form.title,
      description: form.description,
      assignee_employee_id: form.assignee_employee_id,
      priority: form.priority,
      due_date: form.due_date,
      estimated_hours: Number(form.estimated_hours) || 0,
    }
    try {
      if (editingId) {
        await apiClient.put(`/api/project/tasks/${editingId}`, { ...payload, status: form.status })
      } else {
        await apiClient.post('/api/project/tasks', {
          ...payload,
          branch_id: branchId || null,
          project_id: form.project_id,
        })
      }
      setEditing(false)
      loadTasks(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data tugas')
    } finally {
      setSaving(false)
    }
  }

  const projectLabel = (id) => {
    const p = projects.find((x) => x.id === id)
    return p ? `${p.project_code} — ${p.name}` : '-'
  }

  const columns = [
    { key: 'task_number', label: 'Nomor', render: (t) => <code>{t.task_number}</code> },
    {
      key: 'title',
      label: 'Tugas',
      render: (t) => (
        <div>
          <div>{t.title}</div>
          <div className="text-secondary small">{projectLabel(t.project_id)}</div>
        </div>
      ),
    },
    { key: 'assignee_name', label: 'Penanggung Jawab', render: (t) => t.assignee_name ?? <span className="text-secondary">Belum ditugaskan</span> },
    {
      key: 'priority',
      label: 'Prioritas',
      render: (t) => <span className={`badge ${PRIORITY_BADGE[t.priority] ?? 'text-bg-secondary'}`}>{t.priority}</span>,
    },
    {
      key: 'due_date',
      label: 'Tenggat',
      render: (t) => (t.due_date ? new Date(t.due_date).toLocaleDateString('id-ID') : '-'),
    },
    {
      key: 'estimated_hours',
      label: 'Estimasi',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (t) => `${new Intl.NumberFormat('id-ID').format(t.estimated_hours ?? 0)} jam`,
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
      render: (t) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(t)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  const openProjects = projects.filter((p) => OPEN_PROJECT_STATUSES.includes(p.status))

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Tugas</h2>
          <div className="text-secondary small">
            Proyek tidak bisa ditutup selagi masih ada tugas berstatus TODO atau IN_PROGRESS.
          </div>
        </div>
        <button
          type="button"
          className="btn btn-primary btn-sm"
          disabled={!companyId || openProjects.length === 0}
          onClick={openCreate}
        >
          <i className="bi bi-plus-lg me-1" />
          Tambah Tugas
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {companyId && openProjects.length === 0 && !loading && (
        <div className="alert alert-info py-2 small">Belum ada proyek yang terbuka. Buat proyek dulu di menu Proyek.</div>
      )}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={tasks}
          loading={loading}
          searchPlaceholder="Cari nomor, judul tugas, atau penanggung jawab..."
          emptyMessage="Belum ada tugas."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Tugas' : 'Tambah Tugas'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="task-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="task-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-12">
                <label className="form-label">Proyek</label>
                <select
                  className="form-select"
                  value={form.project_id}
                  onChange={(e) => setForm({ ...form, project_id: e.target.value })}
                  disabled={!!editingId}
                  required
                >
                  <option value="">Pilih proyek...</option>
                  {(editingId ? projects : openProjects).map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.project_code} — {p.name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-12">
                <label className="form-label">Judul</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.title}
                  onChange={(e) => setForm({ ...form, title: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Penanggung Jawab</label>
                <select
                  className="form-select"
                  value={form.assignee_employee_id}
                  onChange={(e) => setForm({ ...form, assignee_employee_id: e.target.value })}
                >
                  <option value="">(belum ditugaskan)</option>
                  {employees.map((emp) => (
                    <option key={emp.id} value={emp.id}>
                      {emp.employee_code} — {emp.first_name} {emp.last_name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Prioritas</label>
                <select
                  className="form-select"
                  value={form.priority}
                  onChange={(e) => setForm({ ...form, priority: e.target.value })}
                >
                  {PRIORITIES.map((p) => (
                    <option key={p} value={p}>{p}</option>
                  ))}
                </select>
              </div>
              <div className="col-4">
                <label className="form-label">Tenggat</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.due_date}
                  onChange={(e) => setForm({ ...form, due_date: e.target.value })}
                />
              </div>
              <div className="col-4">
                <label className="form-label">Estimasi (jam)</label>
                <input
                  type="number"
                  min="0"
                  step="0.5"
                  className="form-control"
                  value={form.estimated_hours}
                  onChange={(e) => setForm({ ...form, estimated_hours: e.target.value })}
                />
              </div>
              {editingId && (
                <div className="col-4">
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
              )}
              <div className="col-12">
                <label className="form-label">Deskripsi</label>
                <textarea
                  className="form-control"
                  rows={3}
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

export default TasksPage
