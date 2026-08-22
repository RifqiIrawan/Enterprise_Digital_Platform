import { useEffect, useMemo, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

function LeaveQuotaPage() {
  const { companyId } = useCompany()
  const { can } = usePagePermission()
  const currentYear = new Date().getFullYear()
  const [year, setYear] = useState(currentYear)
  const [quotas, setQuotas] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(null)
  const [form, setForm] = useState({ total_days: 12, carried_over: 0, note: '' })
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function load() {
    if (!companyId) {
      setLoading(false)
      return
    }
    setLoading(true)
    apiClient
      .get('/api/hr/leave-quotas', { params: { company_id: companyId, year } })
      .then(({ data }) => setQuotas(data))
      .catch(() => setError('Gagal memuat kuota cuti. Pastikan hr-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(load, [companyId, year])

  const yearOptions = useMemo(() => [currentYear - 1, currentYear, currentYear + 1], [currentYear])

  function openEdit(q) {
    setForm({ total_days: q.total_days, carried_over: q.carried_over, note: q.note ?? '' })
    setFormError('')
    setEditing(q)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.put('/api/hr/leave-quotas', {
        employee_id: editing.employee_id,
        year,
        total_days: Number(form.total_days),
        carried_over: Number(form.carried_over),
        note: form.note,
      })
      setEditing(null)
      load()
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan kuota cuti')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'employee_name', label: 'Karyawan' },
    {
      key: 'total_days',
      label: 'Jatah',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (q) => (
        <>
          {q.total_days}
          {q.carried_over > 0 && <span className="text-secondary small"> + {q.carried_over} sisa lalu</span>}
          {!q.has_quota_row && <span className="badge text-bg-light border ms-2">default</span>}
        </>
      ),
    },
    {
      key: 'used_days',
      label: 'Terpakai',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (q) => q.used_days,
    },
    {
      key: 'remaining_days',
      label: 'Sisa',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (q) => (
        <span className={q.remaining_days <= 0 ? 'text-danger fw-semibold' : 'fw-semibold'}>
          {q.remaining_days}
        </span>
      ),
    },
    { key: 'note', label: 'Catatan', cellClassName: 'text-secondary small', maxWidth: 240 },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (q) =>
        can('update') && (
          <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(q)}>
            <i className="bi bi-pencil me-1" />
            Atur
          </button>
        ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Kuota Cuti</h2>
          <div className="text-secondary small">
            Jatah cuti tahunan per karyawan. Hari terpakai dihitung dari cuti <strong>tahunan</strong> yang
            sudah disetujui &mdash; cuti sakit, melahirkan, dan tanpa gaji tidak memotong jatah, begitu juga
            tanggal merah yang jatuh di tengah rentang cuti.
          </div>
        </div>
        <div className="d-flex align-items-center gap-2">
          <label className="form-label mb-0 small text-secondary">Tahun</label>
          <select
            className="form-select form-select-sm"
            style={{ width: 120 }}
            value={year}
            onChange={(e) => setYear(Number(e.target.value))}
          >
            {yearOptions.map((y) => (
              <option key={y} value={y}>
                {y}
              </option>
            ))}
          </select>
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {!loading && !companyId && (
        <div className="alert alert-warning py-2 small">Pilih company dulu untuk melihat kuota cuti.</div>
      )}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={quotas}
          loading={loading}
          searchPlaceholder="Cari karyawan..."
          emptyMessage="Belum ada karyawan aktif."
        />
      </div>

      {editing && (
        <Modal
          title={`Kuota Cuti ${year} — ${editing.employee_name}`}
          onClose={() => setEditing(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(null)}>
                Batal
              </button>
              <button type="submit" form="quota-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="quota-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="alert alert-light border py-2 small mb-0">
              Sudah terpakai <strong>{editing.used_days}</strong> hari di tahun {year}. Menurunkan jatah di
              bawah angka itu tidak membatalkan cuti yang sudah disetujui &mdash; sisanya akan tampil negatif
              supaya selisihnya kelihatan.
            </div>
            <div>
              <label className="form-label">Jatah tahun ini (hari)</label>
              <input
                type="number"
                min={0}
                className="form-control"
                value={form.total_days}
                onChange={(e) => setForm({ ...form, total_days: e.target.value })}
                required
              />
              <div className="form-text">Minimum menurut UU Ketenagakerjaan: 12 hari.</div>
            </div>
            <div>
              <label className="form-label">Sisa tahun lalu yang dibawa (hari)</label>
              <input
                type="number"
                min={0}
                className="form-control"
                value={form.carried_over}
                onChange={(e) => setForm({ ...form, carried_over: e.target.value })}
              />
            </div>
            <div>
              <label className="form-label">Catatan</label>
              <input
                type="text"
                className="form-control"
                placeholder="mis. tambahan masa kerja 5 tahun"
                value={form.note}
                onChange={(e) => setForm({ ...form, note: e.target.value })}
              />
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default LeaveQuotaPage
