import { useCallback, useEffect, useState } from 'react'
import apiClient from '../../../services/apiClient.js'
import Modal from '../../../components/common/Modal.jsx'
import DataTable from '../../../components/common/DataTable.jsx'
import { useCompany } from '../../../store/CompanyContext.jsx'
import { usePagePermission } from '../../../store/PermissionContext.jsx'

const emptyForm = { code: '', name: '', branch_id: '', status: 'active' }

const STATUS_BADGE = {
  active: 'text-bg-success',
  inactive: 'text-bg-secondary',
}

function DepartmentListPage() {
  // Berbeda dari Company/Branch, department TIDAK ada di CompanyContext
  // (switcher tidak memakainya), jadi halaman ini memang fetch sendiri.
  const { companies, branches, companyId } = useCompany()
  const { can } = usePagePermission()
  const company = companies.find((c) => c.id === companyId)

  const [departments, setDepartments] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const [deleting, setDeleting] = useState(null)
  const [deleteError, setDeleteError] = useState('')
  const [deleteBusy, setDeleteBusy] = useState(false)

  const loadDepartments = useCallback((cid) => {
    if (!cid) {
      setDepartments([])
      setLoading(false)
      return Promise.resolve()
    }
    setLoading(true)
    return apiClient
      .get(`/api/company/companies/${cid}/departments`)
      .then(({ data }) => {
        setDepartments(data)
        setError('')
      })
      .catch(() => setError('Gagal memuat data department. Pastikan company-service aktif.'))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    loadDepartments(companyId)
  }, [companyId, loadDepartments])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(d) {
    setEditingId(d.id)
    setForm({ code: d.code, name: d.name, branch_id: d.branch_id ?? '', status: d.status })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    // String kosong berarti company-wide; backend menormalkannya jadi NULL,
    // tapi dikirim apa adanya supaya niatnya terbaca di payload.
    const branchID = form.branch_id || null
    try {
      if (editingId) {
        await apiClient.put(`/api/company/companies/${companyId}/departments/${editingId}`, {
          name: form.name,
          branch_id: branchID,
          status: form.status,
        })
      } else {
        await apiClient.post(`/api/company/companies/${companyId}/departments`, {
          code: form.code,
          name: form.name,
          branch_id: branchID,
        })
      }
      await loadDepartments(companyId)
      setEditing(false)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data department')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    setDeleteBusy(true)
    setDeleteError('')
    try {
      await apiClient.delete(`/api/company/companies/${companyId}/departments/${deleting.id}`)
      await loadDepartments(companyId)
      setDeleting(null)
    } catch (err) {
      setDeleteError(err.response?.data?.error ?? 'Gagal menghapus department')
    } finally {
      setDeleteBusy(false)
    }
  }

  const branchName = (id) => branches.find((b) => b.id === id)?.name

  const columns = [
    { key: 'code', label: 'Kode', render: (d) => <code>{d.code}</code> },
    { key: 'name', label: 'Nama Department' },
    {
      key: 'branch_id',
      label: 'Branch',
      searchValue: (d) => branchName(d.branch_id) ?? 'company-wide',
      render: (d) =>
        d.branch_id ? (
          branchName(d.branch_id) ?? <code className="small">{d.branch_id.slice(0, 8)}</code>
        ) : (
          <span className="text-secondary">Company-wide</span>
        ),
    },
    {
      key: 'status',
      label: 'Status',
      render: (d) => <span className={`badge ${STATUS_BADGE[d.status] ?? 'text-bg-secondary'}`}>{d.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (d) => (
        <div className="d-flex gap-1 justify-content-end">
          {can('update') && (
            <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(d)}>
              <i className="bi bi-pencil" />
            </button>
          )}
          <button
            type="button"
            className="btn btn-sm btn-outline-danger"
            onClick={() => {
              setDeleting(d)
              setDeleteError('')
            }}
          >
            <i className="bi bi-trash" />
          </button>
        </div>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Department Management</h2>
          <div className="text-secondary small">
            Department dari company {company ? <strong>{company.name}</strong> : 'yang dipilih'}. Department tanpa
            branch berlaku company-wide.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Department
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={departments}
          loading={loading}
          searchPlaceholder="Cari kode, nama department, atau branch..."
          emptyMessage="Belum ada department di company ini."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Department' : 'Tambah Department'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="department-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="department-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-4">
                <label className="form-label">Kode</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.code}
                  onChange={(e) => setForm({ ...form, code: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-8">
                <label className="form-label">Nama Department</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              <div className="col-8">
                <label className="form-label">Branch</label>
                <select
                  className="form-select"
                  value={form.branch_id}
                  onChange={(e) => setForm({ ...form, branch_id: e.target.value })}
                >
                  <option value="">(company-wide)</option>
                  {branches.map((b) => (
                    <option key={b.id} value={b.id}>
                      {b.code} — {b.name}
                    </option>
                  ))}
                </select>
              </div>
              {editingId && (
                <div className="col-4">
                  <label className="form-label">Status</label>
                  <select
                    className="form-select"
                    value={form.status}
                    onChange={(e) => setForm({ ...form, status: e.target.value })}
                  >
                    <option value="active">active</option>
                    <option value="inactive">inactive</option>
                  </select>
                </div>
              )}
            </div>
          </form>
        </Modal>
      )}

      {deleting && (
        <Modal
          title="Hapus Department"
          onClose={() => setDeleting(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setDeleting(null)}>
                Batal
              </button>
              <button type="button" className="btn btn-danger" disabled={deleteBusy} onClick={handleDelete}>
                {deleteBusy ? 'Menghapus...' : 'Hapus Permanen'}
              </button>
            </>
          }
        >
          <div className="d-flex flex-column gap-3">
            {deleteError && <div className="alert alert-danger py-2 small mb-0">{deleteError}</div>}
            <p className="mb-0">
              Hapus department <strong>{deleting.name}</strong> ({deleting.code}) secara permanen?
            </p>
            <div className="alert alert-warning py-2 small mb-0">
              Data karyawan di modul HR yang menunjuk department ini akan menyimpan referensi yang tidak bisa ditelusuri
              lagi. Untuk sekadar menghentikan pemakaian baru, pakai <strong>Edit → status inactive</strong>.
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

export default DepartmentListPage
