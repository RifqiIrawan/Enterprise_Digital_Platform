import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = {
  project_code: '',
  name: '',
  description: '',
  customer_name: '',
  manager_employee_id: '',
  start_date: '',
  end_date: '',
  budget_amount: '',
  notes: '',
}

const STATUS_BADGE = {
  PLANNING: 'text-bg-secondary',
  ACTIVE: 'text-bg-primary',
  ON_HOLD: 'text-bg-warning',
  COMPLETED: 'text-bg-success',
  CANCELLED: 'text-bg-danger',
}

// Status proyek hanya berpindah lewat endpoint transisi khusus (tidak ada PUT
// status), jadi tombolnya dipetakan per status saat ini -- pola yang sama
// dengan DeliveryOrdersPage dan OrdersPage e-commerce.
const STATUS_ACTIONS = {
  PLANNING: [
    { action: 'activate', label: 'Aktifkan', className: 'btn-outline-primary', icon: 'bi-play-fill' },
    { action: 'cancel', label: 'Batalkan', className: 'btn-outline-danger', icon: 'bi-x-lg' },
  ],
  ACTIVE: [
    { action: 'hold', label: 'Tahan', className: 'btn-outline-warning', icon: 'bi-pause-fill' },
    { action: 'complete', label: 'Selesai', className: 'btn-outline-success', icon: 'bi-check-lg' },
    { action: 'cancel', label: 'Batalkan', className: 'btn-outline-danger', icon: 'bi-x-lg' },
  ],
  ON_HOLD: [
    { action: 'activate', label: 'Lanjutkan', className: 'btn-outline-primary', icon: 'bi-play-fill' },
    { action: 'cancel', label: 'Batalkan', className: 'btn-outline-danger', icon: 'bi-x-lg' },
  ],
}

const currency = (v) => new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(v ?? 0)

function ProjectsPage() {
  const { companyId, branchId } = useCompany()
  const [projects, setProjects] = useState([])
  const [employees, setEmployees] = useState([])
  const [accounts, setAccounts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionError, setActionError] = useState('')
  const [busyId, setBusyId] = useState(null)

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const [postingProject, setPostingProject] = useState(null)
  const [postForm, setPostForm] = useState({ expense_account_id: '', payable_account_id: '', entry_date: '' })
  const [postError, setPostError] = useState('')
  const [postSaving, setPostSaving] = useState(false)

  function loadProjects(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/project/projects', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setProjects(data))
      .catch(() => setError('Gagal memuat data proyek. Pastikan project-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadProjects(companyId, branchId)
    // Manajer proyek hanya boleh karyawan ACTIVE -- backend menolak yang lain,
    // jadi dropdown-nya pun disaring supaya tidak menawarkan pilihan yang pasti
    // gagal (pola yang sama seperti AttendancePage).
    apiClient
      .get('/api/hr/employees', { params: { company_id: companyId, status: 'ACTIVE' } })
      .then(({ data }) => setEmployees(data))
      .catch(() => setEmployees([]))
    apiClient
      .get('/api/finance/accounts', { params: { company_id: companyId } })
      .then(({ data }) => setAccounts(data))
      .catch(() => setAccounts([]))
  }, [companyId, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(p) {
    setEditingId(p.id)
    setForm({
      project_code: p.project_code,
      name: p.name,
      description: p.description ?? '',
      customer_name: p.customer_name ?? '',
      manager_employee_id: p.manager_employee_id ?? '',
      start_date: (p.start_date ?? '').slice(0, 10),
      end_date: (p.end_date ?? '').slice(0, 10),
      budget_amount: p.budget_amount ?? '',
      notes: p.notes ?? '',
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
      name: form.name,
      description: form.description,
      customer_name: form.customer_name,
      manager_employee_id: form.manager_employee_id,
      start_date: form.start_date,
      end_date: form.end_date,
      budget_amount: Number(form.budget_amount) || 0,
      notes: form.notes,
    }
    try {
      if (editingId) {
        await apiClient.put(`/api/project/projects/${editingId}`, payload)
      } else {
        await apiClient.post('/api/project/projects', {
          ...payload,
          branch_id: branchId || null,
          project_code: form.project_code,
        })
      }
      setEditing(false)
      loadProjects(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data proyek')
    } finally {
      setSaving(false)
    }
  }

  async function runAction(project, action) {
    setActionError('')
    setBusyId(project.id)
    try {
      await apiClient.post(`/api/project/projects/${project.id}/${action}`)
      loadProjects(companyId, branchId)
    } catch (err) {
      setActionError(err.response?.data?.error ?? `Gagal menjalankan aksi ${action}`)
    } finally {
      setBusyId(null)
    }
  }

  function openPost(p) {
    setPostingProject(p)
    setPostForm({ expense_account_id: '', payable_account_id: '', entry_date: '' })
    setPostError('')
  }

  async function handlePostCost(e) {
    e.preventDefault()
    setPostSaving(true)
    setPostError('')
    try {
      await apiClient.post(`/api/project/projects/${postingProject.id}/post-cost`, {
        company_id: companyId,
        ...postForm,
      })
      setPostingProject(null)
      loadProjects(companyId, branchId)
    } catch (err) {
      setPostError(err.response?.data?.error ?? 'Gagal memposting biaya proyek ke GL')
    } finally {
      setPostSaving(false)
    }
  }

  const columns = [
    { key: 'project_code', label: 'Kode', render: (p) => <code>{p.project_code}</code> },
    {
      key: 'name',
      label: 'Proyek',
      render: (p) => (
        <div>
          <div>{p.name}</div>
          {p.customer_name && <div className="text-secondary small">{p.customer_name}</div>}
        </div>
      ),
    },
    { key: 'manager_name', label: 'Manajer', render: (p) => p.manager_name ?? <span className="text-secondary">-</span> },
    {
      key: 'budget_amount',
      label: 'Anggaran',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (p) => currency(p.budget_amount),
    },
    {
      key: 'actual_cost',
      label: 'Realisasi',
      className: 'text-end',
      cellClassName: 'text-end',
      // Realisasi di atas anggaran ditandai merah. Angkanya berasal dari
      // timesheet yang benar-benar sudah masuk jurnal finance-service, bukan
      // input manual.
      render: (p) => (
        <span className={p.budget_amount > 0 && p.actual_cost > p.budget_amount ? 'text-danger fw-semibold' : ''}>
          {currency(p.actual_cost)}
        </span>
      ),
    },
    {
      key: 'status',
      label: 'Status',
      render: (p) => <span className={`badge ${STATUS_BADGE[p.status] ?? 'text-bg-secondary'}`}>{p.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (p) => (
        <div className="d-flex gap-1 justify-content-end flex-wrap">
          {p.status !== 'COMPLETED' && p.status !== 'CANCELLED' && (
            <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(p)}>
              <i className="bi bi-pencil" />
            </button>
          )}
          {p.status === 'ACTIVE' && (
            <button type="button" className="btn btn-sm btn-outline-dark" onClick={() => openPost(p)}>
              <i className="bi bi-journal-arrow-up me-1" />
              Posting Biaya
            </button>
          )}
          {(STATUS_ACTIONS[p.status] ?? []).map((a) => (
            <button
              key={a.action}
              type="button"
              className={`btn btn-sm ${a.className}`}
              disabled={busyId === p.id}
              onClick={() => runAction(p, a.action)}
            >
              <i className={`bi ${a.icon} me-1`} />
              {a.label}
            </button>
          ))}
        </div>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Proyek</h2>
          <div className="text-secondary small">
            Realisasi biaya hanya bertambah lewat posting timesheet yang sudah disetujui ke GL.
          </div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Proyek
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {actionError && <div className="alert alert-warning py-2 small">{actionError}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={projects}
          loading={loading}
          searchPlaceholder="Cari kode, nama proyek, atau pelanggan..."
          emptyMessage="Belum ada proyek."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Proyek' : 'Tambah Proyek'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="project-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="project-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Proyek</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.project_code}
                  onChange={(e) => setForm({ ...form, project_code: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Nama Proyek</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Pelanggan</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.customer_name}
                  onChange={(e) => setForm({ ...form, customer_name: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Manajer Proyek</label>
                <select
                  className="form-select"
                  value={form.manager_employee_id}
                  onChange={(e) => setForm({ ...form, manager_employee_id: e.target.value })}
                >
                  <option value="">(belum ditentukan)</option>
                  {employees.map((emp) => (
                    <option key={emp.id} value={emp.id}>
                      {emp.employee_code} — {emp.first_name} {emp.last_name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-4">
                <label className="form-label">Mulai</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.start_date}
                  onChange={(e) => setForm({ ...form, start_date: e.target.value })}
                />
              </div>
              <div className="col-4">
                <label className="form-label">Target Selesai</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.end_date}
                  onChange={(e) => setForm({ ...form, end_date: e.target.value })}
                />
              </div>
              <div className="col-4">
                <label className="form-label">Anggaran</label>
                <input
                  type="number"
                  min="0"
                  step="0.01"
                  className="form-control"
                  value={form.budget_amount}
                  onChange={(e) => setForm({ ...form, budget_amount: e.target.value })}
                />
              </div>
              <div className="col-12">
                <label className="form-label">Deskripsi</label>
                <textarea
                  className="form-control"
                  rows={2}
                  value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                />
              </div>
              <div className="col-12">
                <label className="form-label">Catatan</label>
                <textarea
                  className="form-control"
                  rows={2}
                  value={form.notes}
                  onChange={(e) => setForm({ ...form, notes: e.target.value })}
                />
              </div>
            </div>
          </form>
        </Modal>
      )}

      {postingProject && (
        <Modal
          title={`Posting Biaya — ${postingProject.project_code}`}
          onClose={() => setPostingProject(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setPostingProject(null)}>
                Batal
              </button>
              <button type="submit" form="post-cost-form" className="btn btn-primary" disabled={postSaving}>
                {postSaving ? 'Memposting...' : 'Posting ke GL'}
              </button>
            </>
          }
        >
          <form id="post-cost-form" onSubmit={handlePostCost} className="d-flex flex-column gap-3">
            {postError && <div className="alert alert-danger py-2 small mb-0">{postError}</div>}
            <div className="alert alert-info py-2 small mb-0">
              Semua timesheet berstatus <strong>APPROVED</strong> pada proyek ini akan dijadikan satu jurnal di
              finance-service, lalu ditandai POSTED.
            </div>
            <div className="row g-3">
              <div className="col-12">
                <label className="form-label">Akun Beban (debit)</label>
                <select
                  className="form-select"
                  value={postForm.expense_account_id}
                  onChange={(e) => setPostForm({ ...postForm, expense_account_id: e.target.value })}
                  required
                >
                  <option value="">Pilih akun...</option>
                  {accounts.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.account_code} — {a.account_name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-12">
                <label className="form-label">Akun Hutang / Akrual (kredit)</label>
                <select
                  className="form-select"
                  value={postForm.payable_account_id}
                  onChange={(e) => setPostForm({ ...postForm, payable_account_id: e.target.value })}
                  required
                >
                  <option value="">Pilih akun...</option>
                  {accounts.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.account_code} — {a.account_name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Jurnal</label>
                <input
                  type="date"
                  className="form-control"
                  value={postForm.entry_date}
                  onChange={(e) => setPostForm({ ...postForm, entry_date: e.target.value })}
                />
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default ProjectsPage
