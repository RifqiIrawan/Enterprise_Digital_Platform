import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { reference_type: 'LEAD', reference_id: '', activity_type: 'CALL', subject: '', description: '', due_date: '' }

const ACTIVITY_TYPE_BADGE = {
  CALL: 'text-bg-info',
  EMAIL: 'text-bg-primary',
  MEETING: 'text-bg-warning',
  NOTE: 'text-bg-secondary',
  TASK: 'text-bg-success',
}

function ActivitiesPage() {
  const { companyId, branchId } = useCompany()
  const [leads, setLeads] = useState([])
  const [accounts, setAccounts] = useState([])
  const [contacts, setContacts] = useState([])
  const [opportunities, setOpportunities] = useState([])
  const [activities, setActivities] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [isDone, setIsDone] = useState(false)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadActivities(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/crm/activities', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setActivities(data))
      .catch(() => setError('Gagal memuat data activity. Pastikan crm-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadActivities(companyId, branchId)
    apiClient.get('/api/crm/leads', { params: { company_id: companyId } }).then(({ data }) => setLeads(data))
    apiClient.get('/api/crm/accounts', { params: { company_id: companyId } }).then(({ data }) => setAccounts(data))
    apiClient.get('/api/crm/contacts', { params: { company_id: companyId } }).then(({ data }) => setContacts(data))
    apiClient.get('/api/crm/opportunities', { params: { company_id: companyId } }).then(({ data }) => setOpportunities(data))
  }, [companyId, branchId])

  // referenceOptionsFor/referenceLabel keep the 4 polymorphic lookup lists
  // (leads/accounts/contacts/opportunities) in one place so both the form's
  // dropdown and the table's "Terkait" column resolve names the same way.
  function referenceOptionsFor(referenceType) {
    switch (referenceType) {
      case 'LEAD':
        return leads.map((l) => ({ id: l.id, label: `${l.first_name} ${l.last_name} (${l.lead_number})` }))
      case 'ACCOUNT':
        return accounts.map((a) => ({ id: a.id, label: a.name }))
      case 'CONTACT':
        return contacts.map((c) => ({ id: c.id, label: `${c.first_name} ${c.last_name}` }))
      case 'OPPORTUNITY':
        return opportunities.map((o) => ({ id: o.id, label: o.name }))
      default:
        return []
    }
  }

  function referenceLabel(referenceType, referenceId) {
    const match = referenceOptionsFor(referenceType).find((opt) => opt.id === referenceId)
    return match ? match.label : referenceId
  }

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setIsDone(false)
    setFormError('')
    setEditing(true)
  }

  function openEdit(a) {
    setEditingId(a.id)
    setForm({
      reference_type: a.reference_type,
      reference_id: a.reference_id,
      activity_type: a.activity_type,
      subject: a.subject,
      description: a.description ?? '',
      due_date: a.due_date ? a.due_date.slice(0, 16) : '',
    })
    setIsDone(a.is_done)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/crm/activities/${editingId}`, {
          subject: form.subject,
          description: form.description,
          due_date: form.due_date ? new Date(form.due_date).toISOString() : null,
          is_done: isDone,
        })
      } else {
        await apiClient.post('/api/crm/activities', {
          company_id: companyId,
          branch_id: branchId || null,
          reference_type: form.reference_type,
          reference_id: form.reference_id,
          activity_type: form.activity_type,
          subject: form.subject,
          description: form.description,
          due_date: form.due_date ? new Date(form.due_date).toISOString() : null,
        })
      }
      setEditing(false)
      loadActivities(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data activity')
    } finally {
      setSaving(false)
    }
  }

  async function toggleDone(a) {
    try {
      await apiClient.put(`/api/crm/activities/${a.id}`, {
        subject: a.subject,
        description: a.description,
        due_date: a.due_date,
        is_done: !a.is_done,
      })
      loadActivities(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal memperbarui activity')
    }
  }

  const columns = [
    {
      key: 'subject',
      label: 'Subjek',
      render: (a) => (
        <div>
          <div className={a.is_done ? 'text-decoration-line-through text-secondary' : ''}>{a.subject}</div>
          <div className="text-secondary small">{a.description}</div>
        </div>
      ),
    },
    {
      key: 'reference_type',
      label: 'Terkait',
      render: (a) => (
        <div className="small">
          <span className="text-secondary">{a.reference_type}</span>
          <br />
          {referenceLabel(a.reference_type, a.reference_id)}
        </div>
      ),
    },
    {
      key: 'activity_type',
      label: 'Tipe',
      render: (a) => <span className={`badge ${ACTIVITY_TYPE_BADGE[a.activity_type] ?? 'text-bg-secondary'}`}>{a.activity_type}</span>,
    },
    {
      key: 'activity_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (a) => new Date(a.activity_date).toLocaleString('id-ID'),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (a) => (
        <div className="d-flex gap-1 justify-content-end">
          {a.activity_type === 'TASK' && (
            <button type="button" className="btn btn-sm btn-outline-success" onClick={() => toggleDone(a)}>
              {a.is_done ? 'Buka Lagi' : 'Selesai'}
            </button>
          )}
          <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(a)}>
            <i className="bi bi-pencil" />
          </button>
        </div>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Activities</h2>
          <div className="text-secondary small">Log call/email/meeting/note/task terhadap lead, account, contact, atau opportunity.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Activity
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={activities} loading={loading} searchPlaceholder="Cari subjek activity..." emptyMessage="Belum ada activity." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Activity' : 'Tambah Activity'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="activity-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="activity-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              {!editingId && (
                <>
                  <div className="col-6">
                    <label className="form-label">Terkait Dengan</label>
                    <select
                      className="form-select"
                      value={form.reference_type}
                      onChange={(e) => setForm({ ...form, reference_type: e.target.value, reference_id: '' })}
                    >
                      <option value="LEAD">Lead</option>
                      <option value="ACCOUNT">Account</option>
                      <option value="CONTACT">Contact</option>
                      <option value="OPPORTUNITY">Opportunity</option>
                    </select>
                  </div>
                  <div className="col-6">
                    <label className="form-label">&nbsp;</label>
                    <select
                      className="form-select"
                      value={form.reference_id}
                      onChange={(e) => setForm({ ...form, reference_id: e.target.value })}
                      required
                    >
                      <option value="">Pilih...</option>
                      {referenceOptionsFor(form.reference_type).map((opt) => (
                        <option key={opt.id} value={opt.id}>{opt.label}</option>
                      ))}
                    </select>
                  </div>
                  <div className="col-6">
                    <label className="form-label">Tipe Activity</label>
                    <select className="form-select" value={form.activity_type} onChange={(e) => setForm({ ...form, activity_type: e.target.value })}>
                      <option value="CALL">Call</option>
                      <option value="EMAIL">Email</option>
                      <option value="MEETING">Meeting</option>
                      <option value="NOTE">Note</option>
                      <option value="TASK">Task</option>
                    </select>
                  </div>
                </>
              )}
              <div className={editingId ? 'col-12' : 'col-6'}>
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
                <label className="form-label">Batas Waktu (opsional, untuk Task)</label>
                <input
                  type="datetime-local"
                  className="form-control"
                  value={form.due_date}
                  onChange={(e) => setForm({ ...form, due_date: e.target.value })}
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
                <div className="col-12 form-check">
                  <input
                    type="checkbox"
                    className="form-check-input"
                    id="activity-is-done"
                    checked={isDone}
                    onChange={(e) => setIsDone(e.target.checked)}
                  />
                  <label className="form-check-label" htmlFor="activity-is-done">Selesai</label>
                </div>
              )}
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default ActivitiesPage
