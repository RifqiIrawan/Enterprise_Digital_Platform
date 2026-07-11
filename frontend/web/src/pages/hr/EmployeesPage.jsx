import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const EMPLOYMENT_TYPES = ['PERMANENT', 'CONTRACT', 'INTERN', 'OUTSOURCE']
const PTKP_STATUSES = ['TK/0', 'TK/1', 'TK/2', 'TK/3', 'K/0', 'K/1', 'K/2', 'K/3']
const EMPLOYEE_STATUSES = ['ACTIVE', 'INACTIVE', 'ON_LEAVE', 'TERMINATED']

const emptyForm = {
  employee_code: '',
  first_name: '',
  last_name: '',
  email: '',
  phone: '',
  department: '',
  job_title: '',
  employment_type: 'PERMANENT',
  hire_date: new Date().toISOString().slice(0, 10),
  basic_salary: '',
  monthly_allowance: '',
  ptkp_status: 'TK/0',
  status: 'ACTIVE',
  is_active: true,
}

const STATUS_BADGE = {
  ACTIVE: 'text-bg-success',
  ON_LEAVE: 'text-bg-warning',
  INACTIVE: 'text-bg-secondary',
  TERMINATED: 'text-bg-danger',
}

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

function EmployeesPage() {
  const { companyId } = useCompany()
  const [employees, setEmployees] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadEmployees(cid) {
    setLoading(true)
    apiClient
      .get('/api/hr/employees', { params: { company_id: cid } })
      .then(({ data }) => setEmployees(data))
      .catch(() => setError('Gagal memuat data karyawan. Pastikan hr-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadEmployees(companyId)
  }, [companyId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(emp) {
    setEditingId(emp.id)
    setForm({
      employee_code: emp.employee_code,
      first_name: emp.first_name,
      last_name: emp.last_name ?? '',
      email: emp.email,
      phone: emp.phone ?? '',
      department: emp.department ?? '',
      job_title: emp.job_title ?? '',
      employment_type: emp.employment_type,
      hire_date: emp.hire_date.slice(0, 10),
      basic_salary: emp.basic_salary,
      monthly_allowance: emp.monthly_allowance,
      ptkp_status: emp.ptkp_status,
      status: emp.status,
      is_active: emp.is_active,
    })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/hr/employees/${editingId}`, {
          first_name: form.first_name,
          last_name: form.last_name,
          phone: form.phone,
          department: form.department,
          job_title: form.job_title,
          employment_type: form.employment_type,
          status: form.status,
          basic_salary: Number(form.basic_salary) || 0,
          monthly_allowance: Number(form.monthly_allowance) || 0,
          ptkp_status: form.ptkp_status,
          is_active: form.is_active,
        })
      } else {
        await apiClient.post('/api/hr/employees', {
          company_id: companyId,
          employee_code: form.employee_code,
          first_name: form.first_name,
          last_name: form.last_name,
          email: form.email,
          phone: form.phone,
          department: form.department,
          job_title: form.job_title,
          employment_type: form.employment_type,
          hire_date: form.hire_date,
          basic_salary: Number(form.basic_salary) || 0,
          monthly_allowance: Number(form.monthly_allowance) || 0,
          ptkp_status: form.ptkp_status,
        })
      }
      setEditing(false)
      loadEmployees(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data karyawan')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'employee_code', label: 'Kode', render: (e) => <code>{e.employee_code}</code> },
    {
      key: 'first_name',
      label: 'Nama',
      render: (e) => (
        <div>
          <div>{e.first_name} {e.last_name}</div>
          <div className="text-secondary small">{e.email}</div>
        </div>
      ),
      sortValue: (e) => `${e.first_name} ${e.last_name}`,
    },
    {
      key: 'department',
      label: 'Departemen / Jabatan',
      render: (e) => (
        <div>
          <div>{e.department || '-'}</div>
          <div className="text-secondary small">{e.job_title || '-'}</div>
        </div>
      ),
    },
    { key: 'employment_type', label: 'Tipe', cellClassName: 'text-secondary small' },
    {
      key: 'basic_salary',
      label: 'Gaji Pokok + Tunjangan',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (e) => `${formatMoney(e.basic_salary)} + ${formatMoney(e.monthly_allowance)}`,
      sortValue: (e) => Number(e.basic_salary) + Number(e.monthly_allowance),
    },
    {
      key: 'status',
      label: 'Status',
      render: (e) => <span className={`badge ${STATUS_BADGE[e.status] ?? 'text-bg-secondary'}`}>{e.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (e) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(e)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Data Karyawan</h2>
          <div className="text-secondary small">Master karyawan untuk absensi &amp; payroll.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Karyawan
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={employees}
          loading={loading}
          searchPlaceholder="Cari kode, nama, atau email..."
          emptyMessage="Belum ada karyawan. Tambahkan data karyawan terlebih dahulu."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Karyawan' : 'Tambah Karyawan'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="employee-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="employee-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Karyawan</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.employee_code}
                  onChange={(e) => setForm({ ...form, employee_code: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Email</label>
                <input
                  type="email"
                  className="form-control"
                  value={form.email}
                  onChange={(e) => setForm({ ...form, email: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Nama Depan</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.first_name}
                  onChange={(e) => setForm({ ...form, first_name: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Nama Belakang</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.last_name}
                  onChange={(e) => setForm({ ...form, last_name: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Telepon</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.phone}
                  onChange={(e) => setForm({ ...form, phone: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Masuk</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.hire_date}
                  onChange={(e) => setForm({ ...form, hire_date: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Departemen</label>
                <input
                  type="text"
                  className="form-control"
                  placeholder="mis. Finance"
                  value={form.department}
                  onChange={(e) => setForm({ ...form, department: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Jabatan</label>
                <input
                  type="text"
                  className="form-control"
                  placeholder="mis. Staff Akunting"
                  value={form.job_title}
                  onChange={(e) => setForm({ ...form, job_title: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tipe Karyawan</label>
                <select
                  className="form-select"
                  value={form.employment_type}
                  onChange={(e) => setForm({ ...form, employment_type: e.target.value })}
                >
                  {EMPLOYMENT_TYPES.map((t) => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Status PTKP</label>
                <select
                  className="form-select"
                  value={form.ptkp_status}
                  onChange={(e) => setForm({ ...form, ptkp_status: e.target.value })}
                >
                  {PTKP_STATUSES.map((t) => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Gaji Pokok</label>
                <input
                  type="number"
                  className="form-control"
                  min="0"
                  value={form.basic_salary}
                  onChange={(e) => setForm({ ...form, basic_salary: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tunjangan Bulanan</label>
                <input
                  type="number"
                  className="form-control"
                  min="0"
                  value={form.monthly_allowance}
                  onChange={(e) => setForm({ ...form, monthly_allowance: e.target.value })}
                />
              </div>
              {editingId && (
                <div className="col-6">
                  <label className="form-label">Status</label>
                  <select
                    className="form-select"
                    value={form.status}
                    onChange={(e) => setForm({ ...form, status: e.target.value })}
                  >
                    {EMPLOYEE_STATUSES.map((s) => (
                      <option key={s} value={s}>{s}</option>
                    ))}
                  </select>
                </div>
              )}
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default EmployeesPage
