import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { category_id: '', subject: '', description: '', requester_name: '', requester_email: '', notes: '' }

const OPEN_STATUSES = ['OPEN', 'IN_PROGRESS', 'RESOLVED']

const STATUS_BADGE = {
  OPEN: 'text-bg-secondary',
  IN_PROGRESS: 'text-bg-info',
  RESOLVED: 'text-bg-warning',
  CLOSED: 'text-bg-success',
}

const PRIORITY_BADGE = {
  LOW: 'text-bg-secondary',
  MEDIUM: 'text-bg-info',
  HIGH: 'text-bg-warning',
  URGENT: 'text-bg-danger',
}

function TicketsPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [categories, setCategories] = useState([])
  const [tickets, setTickets] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [priority, setPriority] = useState('MEDIUM')
  const [status, setStatus] = useState('OPEN')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState(null)

  function loadTickets(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/ticketing/tickets', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setTickets(data))
      .catch(() => setError('Gagal memuat data tiket. Pastikan ticketing-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadTickets(companyId, branchId)
    apiClient.get('/api/ticketing/categories', { params: { company_id: companyId } }).then(({ data }) => setCategories(data))
  }, [companyId, branchId])

  const categoryName = (id) => categories.find((c) => c.id === id)?.name ?? id

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setPriority('MEDIUM')
    setStatus('OPEN')
    setFormError('')
    setEditing(true)
  }

  function openEdit(t) {
    setEditingId(t.id)
    setForm({
      category_id: t.category_id,
      subject: t.subject,
      description: t.description ?? '',
      requester_name: t.requester_name,
      requester_email: t.requester_email ?? '',
      notes: t.notes ?? '',
    })
    setPriority(t.priority)
    setStatus(t.status)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/ticketing/tickets/${editingId}`, {
          category_id: form.category_id,
          subject: form.subject,
          description: form.description,
          priority,
          status,
          requester_name: form.requester_name,
          requester_email: form.requester_email,
          notes: form.notes,
        })
      } else {
        await apiClient.post('/api/ticketing/tickets', {
          company_id: companyId,
          branch_id: branchId || null,
          category_id: form.category_id,
          subject: form.subject,
          description: form.description,
          priority,
          requester_name: form.requester_name,
          requester_email: form.requester_email,
          notes: form.notes,
        })
      }
      setEditing(false)
      loadTickets(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data tiket')
    } finally {
      setSaving(false)
    }
  }

  async function handleClose(id) {
    setActingId(id)
    try {
      await apiClient.post(`/api/ticketing/tickets/${id}/close`)
      loadTickets(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal menutup tiket')
    } finally {
      setActingId(null)
    }
  }

  async function handleReopen(id) {
    setActingId(id)
    try {
      await apiClient.post(`/api/ticketing/tickets/${id}/reopen`)
      loadTickets(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal membuka kembali tiket')
    } finally {
      setActingId(null)
    }
  }

  const columns = [
    {
      key: 'subject',
      label: 'Subjek',
      render: (t) => (
        <div>
          <div>{t.subject}</div>
          <div className="text-secondary small">{t.ticket_number}</div>
        </div>
      ),
    },
    { key: 'category_id', label: 'Category', render: (t) => categoryName(t.category_id), sortValue: (t) => categoryName(t.category_id) },
    { key: 'requester_name', label: 'Requester', render: (t) => <div className="small">{t.requester_name}<br />{t.requester_email}</div> },
    {
      key: 'priority',
      label: 'Prioritas',
      render: (t) => <span className={`badge ${PRIORITY_BADGE[t.priority] ?? 'text-bg-secondary'}`}>{t.priority}</span>,
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
        <div className="d-flex gap-1 justify-content-end">
          {can('update') && OPEN_STATUSES.includes(t.status) && (
            <>
              <button type="button" className="btn btn-sm btn-outline-success" disabled={actingId === t.id} onClick={() => handleClose(t.id)}>
                Close
              </button>
              <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(t)}>
                <i className="bi bi-pencil" />
              </button>
            </>
          )}
          {can('update') && t.status === 'CLOSED' && (
            <button type="button" className="btn btn-sm btn-outline-warning" disabled={actingId === t.id} onClick={() => handleReopen(t.id)}>
              Reopen
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
          <h2 className="edp-page-title">Tickets</h2>
          <div className="text-secondary small">Tiket dukungan pelanggan, dari dibuka sampai selesai.</div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId || categories.length === 0} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Ticket
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {categories.length === 0 && !loading && !error && (
        <div className="alert alert-warning py-2 small mb-0">Belum ada category. Tambahkan data category dulu di menu Categories.</div>
      )}

      <div className="card p-3">
        <DataTable columns={columns} data={tickets} loading={loading} searchPlaceholder="Cari subjek atau nomor tiket..." emptyMessage="Belum ada tiket." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Ticket' : 'Tambah Ticket'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="ticket-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="ticket-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-12">
                <label className="form-label">Subjek</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.subject}
                  onChange={(e) => setForm({ ...form, subject: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Category</label>
                <select
                  className="form-select"
                  value={form.category_id}
                  onChange={(e) => setForm({ ...form, category_id: e.target.value })}
                  required
                >
                  <option value="">Pilih category...</option>
                  {categories.map((c) => (
                    <option key={c.id} value={c.id}>{c.category_code} - {c.name}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Prioritas</label>
                <select className="form-select" value={priority} onChange={(e) => setPriority(e.target.value)}>
                  <option value="LOW">Low</option>
                  <option value="MEDIUM">Medium</option>
                  <option value="HIGH">High</option>
                  <option value="URGENT">Urgent</option>
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Nama Requester</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.requester_name}
                  onChange={(e) => setForm({ ...form, requester_name: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Email Requester</label>
                <input
                  type="email"
                  className="form-control"
                  value={form.requester_email}
                  onChange={(e) => setForm({ ...form, requester_email: e.target.value })}
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
              {editingId && (
                <div className="col-6">
                  <label className="form-label">Status</label>
                  <select className="form-select" value={status} onChange={(e) => setStatus(e.target.value)}>
                    {OPEN_STATUSES.map((s) => (
                      <option key={s} value={s}>{s}</option>
                    ))}
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

export default TicketsPage
