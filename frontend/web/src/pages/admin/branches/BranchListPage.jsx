import { useState } from 'react'
import apiClient from '../../../services/apiClient.js'
import Modal from '../../../components/common/Modal.jsx'
import DataTable, { TruncatedText } from '../../../components/common/DataTable.jsx'
import { useCompany } from '../../../store/CompanyContext.jsx'
import { usePagePermission } from '../../../store/PermissionContext.jsx'

const emptyForm = { code: '', name: '', address: '', status: 'active' }

const STATUS_BADGE = {
  active: 'text-bg-success',
  inactive: 'text-bg-secondary',
}

function BranchListPage() {
  const { companies, branches, companyId, loading, reloadBranches } = useCompany()
  const { can } = usePagePermission()
  const company = companies.find((c) => c.id === companyId)

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const [deleting, setDeleting] = useState(null)
  const [deleteError, setDeleteError] = useState('')
  const [deleteBusy, setDeleteBusy] = useState(false)

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(b) {
    setEditingId(b.id)
    setForm({ code: b.code, name: b.name, address: b.address ?? '', status: b.status })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/company/companies/${companyId}/branches/${editingId}`, {
          name: form.name,
          address: form.address,
          status: form.status,
        })
      } else {
        await apiClient.post(`/api/company/companies/${companyId}/branches`, {
          code: form.code,
          name: form.name,
          address: form.address,
        })
      }
      await reloadBranches(companyId)
      setEditing(false)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data branch')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    setDeleteBusy(true)
    setDeleteError('')
    try {
      await apiClient.delete(`/api/company/companies/${companyId}/branches/${deleting.id}`)
      await reloadBranches(companyId)
      setDeleting(null)
    } catch (err) {
      setDeleteError(err.response?.data?.error ?? 'Gagal menghapus branch')
    } finally {
      setDeleteBusy(false)
    }
  }

  const columns = [
    { key: 'code', label: 'Kode', render: (b) => <code>{b.code}</code> },
    { key: 'name', label: 'Nama Branch' },
    {
      key: 'address',
      label: 'Alamat',
      maxWidth: 280,
      render: (b) => <TruncatedText value={b.address} maxWidth={280} />,
      searchValue: (b) => b.address,
    },
    {
      key: 'status',
      label: 'Status',
      render: (b) => <span className={`badge ${STATUS_BADGE[b.status] ?? 'text-bg-secondary'}`}>{b.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (b) => (
        <div className="d-flex gap-1 justify-content-end">
          {can('update') && (
            <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(b)}>
              <i className="bi bi-pencil" />
            </button>
          )}
          <button
            type="button"
            className="btn btn-sm btn-outline-danger"
            onClick={() => {
              setDeleting(b)
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
          <h2 className="edp-page-title">Branch Management</h2>
          <div className="text-secondary small">
            Branch dari company {company ? <strong>{company.name}</strong> : 'yang dipilih'}. Ganti company lewat
            switcher di kanan atas.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Branch
          </button>
        )}
      </div>

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={branches}
          loading={loading}
          searchPlaceholder="Cari kode, nama, atau alamat branch..."
          emptyMessage="Belum ada branch di company ini."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Branch' : 'Tambah Branch'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="branch-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="branch-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
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
                <label className="form-label">Nama Branch</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              <div className="col-12">
                <label className="form-label">Alamat</label>
                <textarea
                  className="form-control"
                  rows={2}
                  value={form.address}
                  onChange={(e) => setForm({ ...form, address: e.target.value })}
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
          title="Hapus Branch"
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
              Hapus branch <strong>{deleting.name}</strong> ({deleting.code}) secara permanen?
            </p>
            {/* Peringatan ini bukan basa-basi: transaksi di modul lain (finance,
                hr, sales, dst) menyimpan branch_id di database masing-masing
                TANPA foreign key ke company-service, jadi tidak ada yang bisa
                menahan atau ikut membersihkannya dari sini. */}
            <div className="alert alert-warning py-2 small mb-0">
              Transaksi yang sudah terlanjur mencatat branch ini di modul lain akan menyimpan referensi yang tidak bisa
              ditelusuri lagi. Untuk sekadar menonaktifkan branch dari pemakaian baru, pakai <strong>Edit → status
              inactive</strong>.
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

export default BranchListPage
