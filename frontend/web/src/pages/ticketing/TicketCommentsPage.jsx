import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { ticket_id: '', comment_text: '' }

function TicketCommentsPage() {
  const { companyId, branchId } = useCompany()
  const [tickets, setTickets] = useState([])
  const [comments, setComments] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [isInternal, setIsInternal] = useState(false)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadComments(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/ticketing/comments', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setComments(data))
      .catch(() => setError('Gagal memuat data comment. Pastikan ticketing-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadComments(companyId, branchId)
    apiClient.get('/api/ticketing/tickets', { params: { company_id: companyId } }).then(({ data }) => setTickets(data))
  }, [companyId, branchId])

  // ticketLabel mirrors CRM's ActivitiesPage referenceLabel idiom, simplified
  // to a single target type (tickets) instead of the 4-way reference_type
  // switch -- comments only ever attach to a ticket here.
  function ticketLabel(ticketId) {
    const match = tickets.find((t) => t.id === ticketId)
    return match ? `${match.ticket_number} — ${match.subject}` : ticketId
  }

  function openCreate() {
    setForm(emptyForm)
    setIsInternal(false)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/ticketing/comments', {
        company_id: companyId,
        branch_id: branchId || null,
        ticket_id: form.ticket_id,
        comment_text: form.comment_text,
        is_internal: isInternal,
      })
      setEditing(false)
      loadComments(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data comment')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    {
      key: 'comment_text',
      label: 'Komentar',
      render: (c) => (
        <div>
          <div>{c.comment_text}</div>
          {c.is_internal && <span className="badge text-bg-secondary mt-1">Internal</span>}
        </div>
      ),
    },
    {
      key: 'ticket_id',
      label: 'Tiket',
      render: (c) => <span className="small">{ticketLabel(c.ticket_id)}</span>,
      sortValue: (c) => ticketLabel(c.ticket_id),
    },
    {
      key: 'created_at',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (c) => new Date(c.created_at).toLocaleString('id-ID'),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Ticket Comments</h2>
          <div className="text-secondary small">Komentar/timeline per tiket, termasuk catatan internal.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId || tickets.length === 0} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Comment
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {tickets.length === 0 && !loading && !error && (
        <div className="alert alert-warning py-2 small mb-0">Belum ada tiket. Tambahkan data tiket dulu di menu Tickets.</div>
      )}

      <div className="card p-3">
        <DataTable columns={columns} data={comments} loading={loading} searchPlaceholder="Cari komentar..." emptyMessage="Belum ada comment." />
      </div>

      {editing && (
        <Modal
          title="Tambah Comment"
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="comment-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="comment-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-12">
                <label className="form-label">Tiket</label>
                <select
                  className="form-select"
                  value={form.ticket_id}
                  onChange={(e) => setForm({ ...form, ticket_id: e.target.value })}
                  required
                >
                  <option value="">Pilih tiket...</option>
                  {tickets.map((t) => (
                    <option key={t.id} value={t.id}>{t.ticket_number} — {t.subject}</option>
                  ))}
                </select>
              </div>
              <div className="col-12">
                <label className="form-label">Komentar</label>
                <textarea
                  className="form-control"
                  rows={3}
                  value={form.comment_text}
                  onChange={(e) => setForm({ ...form, comment_text: e.target.value })}
                  required
                />
              </div>
              <div className="col-12 form-check">
                <input
                  type="checkbox"
                  className="form-check-input"
                  id="comment-is-internal"
                  checked={isInternal}
                  onChange={(e) => setIsInternal(e.target.checked)}
                />
                <label className="form-check-label" htmlFor="comment-is-internal">Catatan internal (tidak terlihat customer)</label>
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default TicketCommentsPage
