import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { account_code: '', name: '', industry: '', website: '', phone: '', email: '', address: '', notes: '' }

const TYPE_BADGE = {
  PROSPECT: 'text-bg-info',
  CUSTOMER: 'text-bg-success',
  PARTNER: 'text-bg-primary',
  OTHER: 'text-bg-secondary',
}

function AccountsPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [accounts, setAccounts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [accountType, setAccountType] = useState('PROSPECT')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [togglingId, setTogglingId] = useState(null)

  function loadAccounts(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/crm/accounts', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setAccounts(data))
      .catch(() => setError('Gagal memuat data account. Pastikan crm-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadAccounts(companyId, branchId)
  }, [companyId, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setAccountType('PROSPECT')
    setFormError('')
    setEditing(true)
  }

  function openEdit(a) {
    setEditingId(a.id)
    setForm({
      account_code: a.account_code,
      name: a.name,
      industry: a.industry ?? '',
      website: a.website ?? '',
      phone: a.phone ?? '',
      email: a.email ?? '',
      address: a.address ?? '',
      notes: a.notes ?? '',
    })
    setAccountType(a.account_type)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/crm/accounts/${editingId}`, {
          name: form.name,
          industry: form.industry,
          website: form.website,
          phone: form.phone,
          email: form.email,
          address: form.address,
          account_type: accountType,
          notes: form.notes,
        })
      } else {
        await apiClient.post('/api/crm/accounts', {
          company_id: companyId,
          branch_id: branchId || null,
          account_code: form.account_code,
          name: form.name,
          industry: form.industry,
          website: form.website,
          phone: form.phone,
          email: form.email,
          address: form.address,
          account_type: accountType,
          notes: form.notes,
        })
      }
      setEditing(false)
      loadAccounts(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data account')
    } finally {
      setSaving(false)
    }
  }

  // Menonaktifkan account ditolak backend kalau masih ada opportunity terbuka --
  // pesannya ditampilkan apa adanya karena sudah menyebut jumlahnya.
  async function toggleStatus(a) {
    const next = a.status === 'ACTIVE' ? 'INACTIVE' : 'ACTIVE'
    if (next === 'INACTIVE' && !window.confirm(`Nonaktifkan account "${a.name}"?`)) return
    setTogglingId(a.id)
    try {
      await apiClient.put(`/api/crm/accounts/${a.id}`, {
        name: a.name,
        industry: a.industry,
        website: a.website,
        phone: a.phone,
        email: a.email,
        address: a.address,
        account_type: a.account_type,
        owner_user_id: a.owner_user_id,
        notes: a.notes,
        status: next,
      })
      loadAccounts(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal mengubah status account')
    } finally {
      setTogglingId(null)
    }
  }

  const columns = [
    { key: 'account_code', label: 'Kode', render: (a) => <code>{a.account_code}</code> },
    {
      key: 'name',
      label: 'Nama',
      render: (a) => (
        <div>
          <div>{a.name}</div>
          <div className="text-secondary small">{a.industry}</div>
        </div>
      ),
    },
    { key: 'phone', label: 'Kontak', render: (a) => <div className="small">{a.phone}<br />{a.email}</div> },
    {
      key: 'account_type',
      label: 'Tipe',
      render: (a) => <span className={`badge ${TYPE_BADGE[a.account_type] ?? 'text-bg-secondary'}`}>{a.account_type}</span>,
    },
    {
      key: 'status',
      label: 'Status',
      render: (a) =>
        a.status === 'ACTIVE' ? (
          <span className="badge text-bg-success">Aktif</span>
        ) : (
          <span className="badge text-bg-secondary">Nonaktif</span>
        ),
      sortValue: (a) => (a.status === 'ACTIVE' ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (a) => (
        <div className="d-flex gap-1 justify-content-end">
          {can('update') && (
            <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(a)}>
              <i className="bi bi-pencil" />
            </button>
          )}
          {can('update') && (
            <button
              type="button"
              className={`btn btn-sm ${a.status === 'ACTIVE' ? 'btn-outline-danger' : 'btn-outline-success'}`}
              disabled={togglingId === a.id}
              onClick={() => toggleStatus(a)}
            >
              {a.status === 'ACTIVE' ? 'Nonaktifkan' : 'Aktifkan'}
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
          <h2 className="edp-page-title">Accounts</h2>
          <div className="text-secondary small">Organisasi yang berhubungan dengan perusahaan (prospek, customer, partner).</div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Account
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={accounts} loading={loading} searchPlaceholder="Cari kode atau nama account..." emptyMessage="Belum ada account." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Account' : 'Tambah Account'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="account-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="account-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Account</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.account_code}
                  onChange={(e) => setForm({ ...form, account_code: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Nama</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Industri</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.industry}
                  onChange={(e) => setForm({ ...form, industry: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tipe</label>
                <select className="form-select" value={accountType} onChange={(e) => setAccountType(e.target.value)}>
                  <option value="PROSPECT">Prospect</option>
                  <option value="CUSTOMER">Customer</option>
                  <option value="PARTNER">Partner</option>
                  <option value="OTHER">Lainnya</option>
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Telepon</label>
                <input type="text" className="form-control" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
              </div>
              <div className="col-6">
                <label className="form-label">Email</label>
                <input type="email" className="form-control" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
              </div>
              <div className="col-6">
                <label className="form-label">Website</label>
                <input type="text" className="form-control" value={form.website} onChange={(e) => setForm({ ...form, website: e.target.value })} />
              </div>
              <div className="col-12">
                <label className="form-label">Alamat</label>
                <input type="text" className="form-control" value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} />
              </div>
              <div className="col-12">
                <label className="form-label">Catatan</label>
                <input type="text" className="form-control" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default AccountsPage
