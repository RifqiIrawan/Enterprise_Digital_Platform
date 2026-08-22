import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { account_id: '', first_name: '', last_name: '', job_title: '', email: '', phone: '', notes: '' }

function ContactsPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [accounts, setAccounts] = useState([])
  const [contacts, setContacts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [isPrimary, setIsPrimary] = useState(false)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [togglingId, setTogglingId] = useState(null)

  function loadContacts(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/crm/contacts', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setContacts(data))
      .catch(() => setError('Gagal memuat data contact. Pastikan crm-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadContacts(companyId, branchId)
    apiClient.get('/api/crm/accounts', { params: { company_id: companyId } }).then(({ data }) => setAccounts(data))
  }, [companyId, branchId])

  // Account nonaktif tidak ditawarkan untuk data BARU, tapi kalau contact yang
  // sedang diedit memang sudah menempel di sana, pilihannya tetap ditampilkan --
  // kalau tidak, menyimpan perubahan kecil apa pun akan diam-diam melepas
  // kaitannya. Daftar lengkapnya tetap dimuat supaya kolom "Account" di tabel
  // tetap menampilkan nama, bukan tanda hubung.
  async function toggleStatus(c) {
    const next = c.status === 'ACTIVE' ? 'INACTIVE' : 'ACTIVE'
    if (next === 'INACTIVE' && !window.confirm(`Nonaktifkan contact "${c.first_name} ${c.last_name ?? ''}"?`)) return
    setTogglingId(c.id)
    try {
      await apiClient.put(`/api/crm/contacts/${c.id}`, {
        account_id: c.account_id,
        first_name: c.first_name,
        last_name: c.last_name,
        job_title: c.job_title,
        email: c.email,
        phone: c.phone,
        is_primary: c.is_primary,
        notes: c.notes,
        status: next,
      })
      loadContacts(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal mengubah status contact')
    } finally {
      setTogglingId(null)
    }
  }

  const selectableAccounts = (currentID) =>
    accounts.filter((a) => a.status === 'ACTIVE' || a.id === currentID)

  const accountName = (id) => {
    if (!id) return '—'
    return accounts.find((a) => a.id === id)?.name ?? id
  }

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setIsPrimary(false)
    setFormError('')
    setEditing(true)
  }

  function openEdit(c) {
    setEditingId(c.id)
    setForm({
      account_id: c.account_id ?? '',
      first_name: c.first_name,
      last_name: c.last_name ?? '',
      job_title: c.job_title ?? '',
      email: c.email ?? '',
      phone: c.phone ?? '',
      notes: c.notes ?? '',
    })
    setIsPrimary(c.is_primary)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const payload = {
        account_id: form.account_id || null,
        first_name: form.first_name,
        last_name: form.last_name,
        job_title: form.job_title,
        email: form.email,
        phone: form.phone,
        is_primary: isPrimary,
        notes: form.notes,
      }
      if (editingId) {
        await apiClient.put(`/api/crm/contacts/${editingId}`, payload)
      } else {
        await apiClient.post('/api/crm/contacts', { ...payload, company_id: companyId, branch_id: branchId || null })
      }
      setEditing(false)
      loadContacts(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data contact')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    {
      key: 'first_name',
      label: 'Nama',
      render: (c) => (
        <div>
          <div>{c.first_name} {c.last_name} {c.is_primary && <span className="badge text-bg-primary ms-1">Primary</span>}</div>
          <div className="text-secondary small">{c.job_title}</div>
        </div>
      ),
    },
    { key: 'account_id', label: 'Account', render: (c) => accountName(c.account_id), sortValue: (c) => accountName(c.account_id) },
    { key: 'phone', label: 'Kontak', render: (c) => <div className="small">{c.phone}<br />{c.email}</div> },
    {
      key: 'status',
      label: 'Status',
      render: (c) =>
        c.status === 'ACTIVE' ? (
          <span className="badge text-bg-success">Aktif</span>
        ) : (
          <span className="badge text-bg-secondary">Nonaktif</span>
        ),
      sortValue: (c) => (c.status === 'ACTIVE' ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (c) => (
        <div className="d-flex gap-1 justify-content-end">
          {can('update') && (
            <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(c)}>
              <i className="bi bi-pencil" />
            </button>
          )}
          {can('update') && (
            <button
              type="button"
              className={`btn btn-sm ${c.status === 'ACTIVE' ? 'btn-outline-danger' : 'btn-outline-success'}`}
              disabled={togglingId === c.id}
              onClick={() => toggleStatus(c)}
            >
              {c.status === 'ACTIVE' ? 'Nonaktifkan' : 'Aktifkan'}
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
          <h2 className="edp-page-title">Contacts</h2>
          <div className="text-secondary small">Orang-orang di dalam sebuah account (opsional, bisa berdiri sendiri tanpa account).</div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Contact
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={contacts} loading={loading} searchPlaceholder="Cari nama contact..." emptyMessage="Belum ada contact." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Contact' : 'Tambah Contact'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="contact-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="contact-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
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
                <input type="text" className="form-control" value={form.last_name} onChange={(e) => setForm({ ...form, last_name: e.target.value })} />
              </div>
              <div className="col-6">
                <label className="form-label">Account (opsional)</label>
                <select className="form-select" value={form.account_id} onChange={(e) => setForm({ ...form, account_id: e.target.value })}>
                  <option value="">Tidak ditentukan</option>
                  {selectableAccounts(form.account_id).map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.account_code} - {a.name}
                      {a.status !== 'ACTIVE' ? ' (nonaktif)' : ''}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Jabatan</label>
                <input type="text" className="form-control" value={form.job_title} onChange={(e) => setForm({ ...form, job_title: e.target.value })} />
              </div>
              <div className="col-6">
                <label className="form-label">Email</label>
                <input type="email" className="form-control" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
              </div>
              <div className="col-6">
                <label className="form-label">Telepon</label>
                <input type="text" className="form-control" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
              </div>
              <div className="col-12 form-check">
                <input
                  type="checkbox"
                  className="form-check-input"
                  id="contact-is-primary"
                  checked={isPrimary}
                  onChange={(e) => setIsPrimary(e.target.checked)}
                />
                <label className="form-check-label" htmlFor="contact-is-primary">Contact utama untuk account ini</label>
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

export default ContactsPage
