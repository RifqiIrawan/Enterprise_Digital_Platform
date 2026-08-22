import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { account_id: '', contact_id: '', name: '', amount: '', probability: '10', expected_close_date: '' }

const OPEN_STAGES = ['PROSPECTING', 'QUALIFICATION', 'PROPOSAL', 'NEGOTIATION']

const STAGE_BADGE = {
  PROSPECTING: 'text-bg-secondary',
  QUALIFICATION: 'text-bg-info',
  PROPOSAL: 'text-bg-primary',
  NEGOTIATION: 'text-bg-warning',
  WON: 'text-bg-success',
  LOST: 'text-bg-danger',
}

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

function OpportunitiesPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [accounts, setAccounts] = useState([])
  const [contacts, setContacts] = useState([])
  const [opportunities, setOpportunities] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [stage, setStage] = useState('PROSPECTING')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState(null)

  function loadOpportunities(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/crm/opportunities', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setOpportunities(data))
      .catch(() => setError('Gagal memuat data opportunity. Pastikan crm-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadOpportunities(companyId, branchId)
    apiClient.get('/api/crm/accounts', { params: { company_id: companyId } }).then(({ data }) => setAccounts(data))
    apiClient.get('/api/crm/contacts', { params: { company_id: companyId } }).then(({ data }) => setContacts(data))
  }, [companyId, branchId])

  // Sama seperti di Contacts: yang nonaktif tidak ditawarkan untuk opportunity
  // baru (backend juga menolaknya), tapi pilihan yang sedang dipakai tetap ada.
  const selectableAccounts = (currentID) =>
    accounts.filter((a) => a.status === 'ACTIVE' || a.id === currentID)

  const accountName = (id) => {
    if (!id) return '—'
    return accounts.find((a) => a.id === id)?.name ?? id
  }
  const contactsForAccount = (accountId) => contacts.filter((c) => c.account_id === accountId)

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setStage('PROSPECTING')
    setFormError('')
    setEditing(true)
  }

  function openEdit(o) {
    setEditingId(o.id)
    setForm({
      account_id: o.account_id,
      contact_id: o.contact_id ?? '',
      name: o.name,
      amount: o.amount ?? '',
      probability: o.probability ?? '0',
      expected_close_date: o.expected_close_date ? o.expected_close_date.slice(0, 10) : '',
    })
    setStage(o.stage)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/crm/opportunities/${editingId}`, {
          contact_id: form.contact_id || null,
          name: form.name,
          stage,
          amount: Number(form.amount) || 0,
          probability: Number(form.probability) || 0,
          expected_close_date: form.expected_close_date || null,
        })
      } else {
        await apiClient.post('/api/crm/opportunities', {
          company_id: companyId,
          branch_id: branchId || null,
          account_id: form.account_id,
          contact_id: form.contact_id || null,
          name: form.name,
          amount: Number(form.amount) || 0,
          probability: Number(form.probability) || 0,
          expected_close_date: form.expected_close_date || null,
        })
      }
      setEditing(false)
      loadOpportunities(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data opportunity')
    } finally {
      setSaving(false)
    }
  }

  async function handleWin(id) {
    setActingId(id)
    try {
      await apiClient.post(`/api/crm/opportunities/${id}/win`)
      loadOpportunities(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal menandai opportunity sebagai won')
    } finally {
      setActingId(null)
    }
  }

  async function handleLose(id) {
    const lostReason = window.prompt('Alasan kalah (opsional):') ?? ''
    setActingId(id)
    try {
      await apiClient.post(`/api/crm/opportunities/${id}/lose`, { lost_reason: lostReason })
      loadOpportunities(companyId, branchId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal menandai opportunity sebagai lost')
    } finally {
      setActingId(null)
    }
  }

  const columns = [
    {
      key: 'name',
      label: 'Nama Deal',
      render: (o) => (
        <div>
          <div>{o.name}</div>
          <div className="text-secondary small">{o.opportunity_number}</div>
        </div>
      ),
    },
    { key: 'account_id', label: 'Account', render: (o) => accountName(o.account_id), sortValue: (o) => accountName(o.account_id) },
    { key: 'amount', label: 'Nilai', className: 'text-end', cellClassName: 'text-end', render: (o) => `Rp ${formatMoney(o.amount)}` },
    { key: 'probability', label: 'Probabilitas', className: 'text-end', cellClassName: 'text-end', render: (o) => `${o.probability}%` },
    {
      key: 'stage',
      label: 'Stage',
      render: (o) => <span className={`badge ${STAGE_BADGE[o.stage] ?? 'text-bg-secondary'}`}>{o.stage}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (o) => (
        <div className="d-flex gap-1 justify-content-end">
          {can('update') && OPEN_STAGES.includes(o.stage) && (
            <>
              <button type="button" className="btn btn-sm btn-outline-success" disabled={actingId === o.id} onClick={() => handleWin(o.id)}>
                Won
              </button>
              <button type="button" className="btn btn-sm btn-outline-danger" disabled={actingId === o.id} onClick={() => handleLose(o.id)}>
                Lost
              </button>
              <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(o)}>
                <i className="bi bi-pencil" />
              </button>
            </>
          )}
        </div>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Opportunities</h2>
          <div className="text-secondary small">Pipeline penjualan per account, dari prospek sampai deal menang/kalah.</div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId || accounts.length === 0} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Opportunity
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {accounts.length === 0 && !loading && !error && (
        <div className="alert alert-warning py-2 small mb-0">Belum ada account. Tambahkan data account dulu di menu Accounts.</div>
      )}

      <div className="card p-3">
        <DataTable columns={columns} data={opportunities} loading={loading} searchPlaceholder="Cari nama deal..." emptyMessage="Belum ada opportunity." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Opportunity' : 'Tambah Opportunity'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="opportunity-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="opportunity-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-12">
                <label className="form-label">Nama Deal</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Account</label>
                <select
                  className="form-select"
                  value={form.account_id}
                  onChange={(e) => setForm({ ...form, account_id: e.target.value, contact_id: '' })}
                  disabled={!!editingId}
                  required
                >
                  <option value="">Pilih account...</option>
                  {selectableAccounts(form.account_id).map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.account_code} - {a.name}
                      {a.status !== 'ACTIVE' ? ' (nonaktif)' : ''}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Contact (opsional)</label>
                <select className="form-select" value={form.contact_id} onChange={(e) => setForm({ ...form, contact_id: e.target.value })}>
                  <option value="">Tidak ditentukan</option>
                  {contactsForAccount(form.account_id).map((c) => (
                    <option key={c.id} value={c.id}>{c.first_name} {c.last_name}</option>
                  ))}
                </select>
              </div>
              <div className="col-4">
                <label className="form-label">Nilai (Rp)</label>
                <input type="number" className="form-control" value={form.amount} onChange={(e) => setForm({ ...form, amount: e.target.value })} min="0" />
              </div>
              <div className="col-4">
                <label className="form-label">Probabilitas (%)</label>
                <input
                  type="number"
                  className="form-control"
                  value={form.probability}
                  onChange={(e) => setForm({ ...form, probability: e.target.value })}
                  min="0"
                  max="100"
                />
              </div>
              <div className="col-4">
                <label className="form-label">Target Closing</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.expected_close_date}
                  onChange={(e) => setForm({ ...form, expected_close_date: e.target.value })}
                />
              </div>
              {editingId && (
                <div className="col-6">
                  <label className="form-label">Stage</label>
                  <select className="form-select" value={stage} onChange={(e) => setStage(e.target.value)}>
                    {OPEN_STAGES.map((s) => (
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

export default OpportunitiesPage
