import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { account_code: '', name: '', industry: '', website: '', phone: '', email: '', address: '', notes: '' }

const TYPE_BADGE = {
  PROSPECT: 'text-bg-info',
  CUSTOMER: 'text-bg-success',
  PARTNER: 'text-bg-primary',
  OTHER: 'text-bg-secondary',
}

function AccountsPage() {
  const { companyId, branchId } = useCompany()
  const [accounts, setAccounts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [accountType, setAccountType] = useState('PROSPECT')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

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
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (a) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(a)}>
          <i className="bi bi-pencil" />
        </button>
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
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Account
        </button>
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
