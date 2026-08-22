import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const LEAVE_TYPES = [
  { value: 'ANNUAL', label: 'Cuti Tahunan' },
  { value: 'SICK', label: 'Sakit' },
  { value: 'MATERNITY', label: 'Melahirkan' },
  { value: 'UNPAID', label: 'Tanpa Gaji' },
  { value: 'OTHER', label: 'Lainnya' },
]

const TYPE_LABEL = Object.fromEntries(LEAVE_TYPES.map((t) => [t.value, t.label]))

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  SUBMITTED: 'text-bg-warning',
  APPROVED: 'text-bg-success',
  REJECTED: 'text-bg-danger',
  CANCELLED: 'text-bg-dark',
}

function currentPeriod() {
  return new Date().toISOString().slice(0, 7)
}

function emptyForm() {
  const today = new Date().toISOString().slice(0, 10)
  return { employee_id: '', leave_type: 'ANNUAL', start_date: today, end_date: today, reason: '' }
}

function LeavePage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [employees, setEmployees] = useState([])
  const [requests, setRequests] = useState([])
  const [period, setPeriod] = useState(currentPeriod())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionError, setActionError] = useState('')
  const [busyId, setBusyId] = useState(null)

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm())
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  // Penolakan wajib beralasan di backend, jadi tombol Tolak membuka modal
  // kecil ini alih-alih langsung mengirim request.
  const [rejecting, setRejecting] = useState(null)
  const [rejectionReason, setRejectionReason] = useState('')

  function loadRequests(cid, p, bid) {
    setLoading(true)
    apiClient
      .get('/api/hr/leave-requests', { params: { company_id: cid, period: p, branch_id: bid } })
      .then(({ data }) => setRequests(data))
      .catch(() => setError('Gagal memuat data cuti. Pastikan hr-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    apiClient
      .get('/api/hr/employees', { params: { company_id: companyId, status: 'ACTIVE' } })
      .then(({ data }) => setEmployees(data))
      .catch(() => setEmployees([]))
  }, [companyId])

  useEffect(() => {
    if (!companyId) return
    loadRequests(companyId, period, branchId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [companyId, period, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm())
    setFormError('')
    setEditing(true)
  }

  function openEdit(request) {
    setEditingId(request.id)
    setForm({
      employee_id: request.employee_id,
      leave_type: request.leave_type,
      start_date: request.start_date.slice(0, 10),
      end_date: request.end_date.slice(0, 10),
      reason: request.reason ?? '',
    })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const payload = {
        company_id: companyId,
        branch_id: branchId || null,
        employee_id: form.employee_id,
        leave_type: form.leave_type,
        start_date: form.start_date,
        end_date: form.end_date,
        reason: form.reason,
      }
      if (editingId) {
        await apiClient.put(`/api/hr/leave-requests/${editingId}`, payload)
      } else {
        await apiClient.post('/api/hr/leave-requests', payload)
      }
      setEditing(false)
      loadRequests(companyId, period, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan pengajuan cuti')
    } finally {
      setSaving(false)
    }
  }

  async function runAction(request, action, body) {
    setActionError('')
    setBusyId(request.id)
    try {
      await apiClient.post(`/api/hr/leave-requests/${request.id}/${action}`, body ?? {})
      loadRequests(companyId, period, branchId)
      return true
    } catch (err) {
      setActionError(err.response?.data?.error ?? `Gagal menjalankan aksi ${action}`)
      return false
    } finally {
      setBusyId(null)
    }
  }

  async function confirmReject(e) {
    e.preventDefault()
    const ok = await runAction(rejecting, 'reject', { rejection_reason: rejectionReason })
    if (ok) setRejecting(null)
  }

  const employeeName = (request) => {
    const emp = employees.find((e) => e.id === request.employee_id)
    return emp ? `${emp.first_name} ${emp.last_name ?? ''}`.trim() : request.employee_name
  }

  const columns = [
    {
      key: 'employee_name',
      label: 'Karyawan',
      render: (l) => (
        <div>
          <div>{employeeName(l)}</div>
          <div className="text-secondary small">{TYPE_LABEL[l.leave_type] ?? l.leave_type}</div>
        </div>
      ),
      sortValue: (l) => employeeName(l),
    },
    {
      key: 'start_date',
      label: 'Periode Cuti',
      cellClassName: 'text-secondary small',
      render: (l) =>
        `${new Date(l.start_date).toLocaleDateString('id-ID')} - ${new Date(l.end_date).toLocaleDateString('id-ID')}`,
    },
    {
      key: 'total_days',
      label: 'Hari Kerja',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (l) => l.total_days,
    },
    {
      key: 'reason',
      label: 'Keterangan',
      cellClassName: 'text-secondary small',
      // Alasan penolakan menggantikan alasan pengajuan begitu ditolak: yang
      // relevan dibaca ulang saat itu adalah kenapa ditolak.
      render: (l) => (l.status === 'REJECTED' ? `Ditolak: ${l.rejection_reason}` : l.reason || '-'),
    },
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
          {l.status === 'DRAFT' && (
            <>
              {can('update') && (
                <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(l)}>
                  <i className="bi bi-pencil" />
                </button>
              )}
              {can('update') && (
                <button
                  type="button"
                  className="btn btn-sm btn-outline-primary"
                  disabled={busyId === l.id}
                  onClick={() => runAction(l, 'submit')}
                >
                  <i className="bi bi-send me-1" />
                  Ajukan
                </button>
              )}
            </>
          )}
          {l.status === 'SUBMITTED' && can('approve') && (
            <button
              type="button"
              className="btn btn-sm btn-outline-success"
              disabled={busyId === l.id}
              onClick={() => runAction(l, 'approve')}
            >
              <i className="bi bi-check-lg me-1" />
              Setujui
            </button>
          )}
          {l.status === 'SUBMITTED' && can('approve') && (
            <button
              type="button"
              className="btn btn-sm btn-outline-danger"
              disabled={busyId === l.id}
              onClick={() => {
                setRejectionReason('')
                setRejecting(l)
              }}
            >
              <i className="bi bi-x-lg me-1" />
              Tolak
            </button>
          )}
          {can('update') && ['DRAFT', 'SUBMITTED', 'APPROVED'].includes(l.status) && (
            <button
              type="button"
              className="btn btn-sm btn-outline-secondary"
              disabled={busyId === l.id}
              onClick={() => runAction(l, 'cancel')}
            >
              Batalkan
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
          <h2 className="edp-page-title">Cuti</h2>
          <div className="text-secondary small">
            Cuti berbayar tetap dihitung hadir saat pro-rata payroll; hanya cuti Tanpa Gaji yang memotong gaji.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Ajukan Cuti
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {actionError && <div className="alert alert-warning py-2 small">{actionError}</div>}

      <div className="card p-3">
        <div className="d-flex align-items-center gap-2 mb-2">
          <label className="form-label mb-0 small text-secondary">Periode</label>
          <input
            type="month"
            className="form-control form-control-sm"
            style={{ maxWidth: 160 }}
            value={period}
            onChange={(e) => setPeriod(e.target.value)}
          />
          <span className="text-secondary small">Cuti yang menyeberang bulan muncul di kedua periode.</span>
        </div>
        <DataTable
          columns={columns}
          data={requests}
          loading={loading}
          searchPlaceholder="Cari karyawan..."
          emptyMessage="Belum ada pengajuan cuti di periode ini."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Pengajuan Cuti' : 'Ajukan Cuti'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="leave-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="leave-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-12">
                <label className="form-label">Karyawan</label>
                <select
                  className="form-select"
                  value={form.employee_id}
                  onChange={(e) => setForm({ ...form, employee_id: e.target.value })}
                  disabled={!!editingId}
                  required
                >
                  <option value="">Pilih karyawan...</option>
                  {employees.map((emp) => (
                    <option key={emp.id} value={emp.id}>
                      {emp.employee_code} - {emp.first_name} {emp.last_name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-12">
                <label className="form-label">Jenis Cuti</label>
                <select
                  className="form-select"
                  value={form.leave_type}
                  onChange={(e) => setForm({ ...form, leave_type: e.target.value })}
                >
                  {LEAVE_TYPES.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </select>
                {form.leave_type === 'UNPAID' && (
                  <div className="form-text text-warning">Cuti tanpa gaji memotong gaji pokok pro-rata.</div>
                )}
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Mulai</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.start_date}
                  onChange={(e) => setForm({ ...form, start_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Selesai</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.end_date}
                  min={form.start_date}
                  onChange={(e) => setForm({ ...form, end_date: e.target.value })}
                  required
                />
              </div>
              <div className="col-12">
                <label className="form-label">Alasan</label>
                <textarea
                  className="form-control"
                  rows={2}
                  value={form.reason}
                  onChange={(e) => setForm({ ...form, reason: e.target.value })}
                />
              </div>
            </div>
          </form>
        </Modal>
      )}

      {rejecting && (
        <Modal
          title="Tolak Pengajuan Cuti"
          onClose={() => setRejecting(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setRejecting(null)}>
                Batal
              </button>
              {can('approve') && (
                <button type="submit" form="leave-reject-form" className="btn btn-danger" disabled={busyId === rejecting.id}>
                  Tolak
                </button>
              )}
            </>
          }
        >
          <form id="leave-reject-form" onSubmit={confirmReject} className="d-flex flex-column gap-2">
            <div className="text-secondary small">
              {employeeName(rejecting)} &middot; {new Date(rejecting.start_date).toLocaleDateString('id-ID')} -{' '}
              {new Date(rejecting.end_date).toLocaleDateString('id-ID')}
            </div>
            <label className="form-label mb-0">Alasan penolakan</label>
            <textarea
              className="form-control"
              rows={3}
              value={rejectionReason}
              onChange={(e) => setRejectionReason(e.target.value)}
              required
            />
          </form>
        </Modal>
      )}
    </div>
  )
}

export default LeavePage
