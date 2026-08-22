import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { first_name: '', last_name: '', company_name: '', email: '', phone: '', notes: '' }

const OPEN_STATUSES = ['NEW', 'CONTACTED', 'QUALIFIED', 'UNQUALIFIED']

const STATUS_BADGE = {
  NEW: 'text-bg-secondary',
  CONTACTED: 'text-bg-info',
  QUALIFIED: 'text-bg-warning',
  UNQUALIFIED: 'text-bg-danger',
  CONVERTED: 'text-bg-success',
}

const SOURCE_LABELS = {
  WEBSITE: 'Website',
  REFERRAL: 'Referral',
  COLD_CALL: 'Cold Call',
  EVENT: 'Event',
  ADVERTISEMENT: 'Iklan',
  SOCIAL_MEDIA: 'Media Sosial',
  OTHER: 'Lainnya',
}

function LeadsPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [leads, setLeads] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [source, setSource] = useState('OTHER')
  const [status, setStatus] = useState('NEW')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [convertingId, setConvertingId] = useState(null)

  function loadLeads(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/crm/leads', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setLeads(data))
      .catch(() => setError('Gagal memuat data lead. Pastikan crm-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadLeads(companyId, branchId)
  }, [companyId, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setSource('OTHER')
    setStatus('NEW')
    setFormError('')
    setEditing(true)
  }

  function openEdit(l) {
    setEditingId(l.id)
    setForm({
      first_name: l.first_name,
      last_name: l.last_name ?? '',
      company_name: l.company_name ?? '',
      email: l.email ?? '',
      phone: l.phone ?? '',
      notes: l.notes ?? '',
    })
    setSource(l.source)
    setStatus(l.status)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/crm/leads/${editingId}`, {
          first_name: form.first_name,
          last_name: form.last_name,
          company_name: form.company_name,
          email: form.email,
          phone: form.phone,
          source,
          status,
          notes: form.notes,
        })
      } else {
        await apiClient.post('/api/crm/leads', {
          company_id: companyId,
          branch_id: branchId || null,
          first_name: form.first_name,
          last_name: form.last_name,
          company_name: form.company_name,
          email: form.email,
          phone: form.phone,
          source,
          notes: form.notes,
        })
      }
      setEditing(false)
      loadLeads(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data lead')
    } finally {
      setSaving(false)
    }
  }

  async function handleConvert(l) {
    if (!window.confirm(`Konversi lead "${l.first_name} ${l.last_name}" menjadi Account, Contact, dan Opportunity?`)) return
    setConvertingId(l.id)
    try {
      const { data } = await apiClient.post(`/api/crm/leads/${l.id}/convert`, {})
      window.alert(`Berhasil dikonversi:\nAccount: ${data.account.name}\nContact: ${data.contact.first_name} ${data.contact.last_name}\nOpportunity: ${data.opportunity.name}`)
      loadLeads(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal mengonversi lead')
    } finally {
      setConvertingId(null)
    }
  }

  const columns = [
    {
      key: 'first_name',
      label: 'Nama',
      render: (l) => (
        <div>
          <div>{l.first_name} {l.last_name}</div>
          <div className="text-secondary small">{l.company_name || l.lead_number}</div>
        </div>
      ),
    },
    { key: 'phone', label: 'Kontak', render: (l) => <div className="small">{l.phone}<br />{l.email}</div> },
    { key: 'source', label: 'Sumber', render: (l) => SOURCE_LABELS[l.source] ?? l.source },
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
        <div className="d-flex gap-1 justify-content-end">
          {can('update') && l.status === 'QUALIFIED' && (
            <button type="button" className="btn btn-sm btn-outline-success" disabled={convertingId === l.id} onClick={() => handleConvert(l)}>
              {convertingId === l.id ? 'Memproses...' : 'Convert'}
            </button>
          )}
          {OPEN_STATUSES.includes(l.status) && can('update') && (
            <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(l)}>
              <i className="bi bi-pencil" />
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
          <h2 className="edp-page-title">Leads</h2>
          <div className="text-secondary small">Prospek awal -- kualifikasi lalu konversi menjadi Account, Contact, dan Opportunity.</div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Lead
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={leads} loading={loading} searchPlaceholder="Cari nama atau perusahaan lead..." emptyMessage="Belum ada lead." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Lead' : 'Tambah Lead'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="lead-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="lead-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
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
                <label className="form-label">Perusahaan</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.company_name}
                  onChange={(e) => setForm({ ...form, company_name: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Sumber</label>
                <select className="form-select" value={source} onChange={(e) => setSource(e.target.value)}>
                  {Object.entries(SOURCE_LABELS).map(([value, label]) => (
                    <option key={value} value={value}>{label}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Email</label>
                <input type="email" className="form-control" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
              </div>
              <div className="col-6">
                <label className="form-label">Telepon</label>
                <input type="text" className="form-control" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
              </div>
              {editingId && (
                <div className="col-6">
                  <label className="form-label">Status</label>
                  <select className="form-select" value={status} onChange={(e) => setStatus(e.target.value)}>
                    <option value="NEW">New</option>
                    <option value="CONTACTED">Contacted</option>
                    <option value="QUALIFIED">Qualified</option>
                    <option value="UNQUALIFIED">Unqualified</option>
                  </select>
                </div>
              )}
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

export default LeadsPage
