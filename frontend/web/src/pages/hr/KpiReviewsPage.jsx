import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  SUBMITTED: 'text-bg-info',
  APPROVED: 'text-bg-success',
  REJECTED: 'text-bg-danger',
}

const RATING_CLASS = {
  'SANGAT BAIK': 'text-success fw-semibold',
  BAIK: 'text-primary fw-semibold',
  CUKUP: 'text-warning fw-semibold',
  'PERLU PERBAIKAN': 'text-danger fw-semibold',
}

function currentPeriod() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

function KpiReviewsPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [reviews, setReviews] = useState([])
  const [employees, setEmployees] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [createForm, setCreateForm] = useState({ employee_id: '', period: currentPeriod() })
  const [createError, setCreateError] = useState('')
  const [saving, setSaving] = useState(false)

  const [detail, setDetail] = useState(null)
  const [detailError, setDetailError] = useState('')
  const [busy, setBusy] = useState(false)
  const [rejecting, setRejecting] = useState(null)
  const [rejectionReason, setRejectionReason] = useState('')

  function load() {
    if (!companyId) {
      setLoading(false)
      return
    }
    setLoading(true)
    Promise.all([
      apiClient.get('/api/hr/kpi-reviews', { params: { company_id: companyId, branch_id: branchId } }),
      apiClient.get('/api/hr/employees', { params: { company_id: companyId, branch_id: branchId } }),
    ])
      .then(([reviewsRes, employeesRes]) => {
        setReviews(reviewsRes.data)
        setEmployees(employeesRes.data)
      })
      .catch(() => setError('Gagal memuat penilaian KPI. Pastikan hr-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(load, [companyId, branchId])

  async function openDetail(review) {
    setDetailError('')
    try {
      const { data } = await apiClient.get(`/api/hr/kpi-reviews/${review.id}`)
      setDetail(data)
    } catch {
      window.alert('Gagal memuat rincian penilaian')
    }
  }

  async function handleCreate(e) {
    e.preventDefault()
    setSaving(true)
    setCreateError('')
    try {
      const { data } = await apiClient.post('/api/hr/kpi-reviews', {
        company_id: companyId,
        branch_id: branchId || null,
        employee_id: createForm.employee_id,
        period: createForm.period,
      })
      setCreating(false)
      load()
      openDetail(data)
    } catch (err) {
      setCreateError(err.response?.data?.error ?? 'Gagal membuat penilaian')
    } finally {
      setSaving(false)
    }
  }

  function setItemValue(indicatorId, value) {
    setDetail((prev) => ({
      ...prev,
      items: prev.items.map((it) =>
        it.indicator_id === indicatorId ? { ...it, actual_value: value } : it
      ),
    }))
  }

  async function saveScores() {
    setBusy(true)
    setDetailError('')
    try {
      await apiClient.put(`/api/hr/kpi-reviews/${detail.id}/scores`, {
        items: detail.items.map((it) => ({
          indicator_id: it.indicator_id,
          actual_value: Number(it.actual_value) || 0,
          note: it.note ?? '',
        })),
        notes: detail.notes ?? '',
      })
      const { data } = await apiClient.get(`/api/hr/kpi-reviews/${detail.id}`)
      setDetail(data)
      load()
    } catch (err) {
      setDetailError(err.response?.data?.error ?? 'Gagal menyimpan nilai')
    } finally {
      setBusy(false)
    }
  }

  async function runAction(action, payload) {
    setBusy(true)
    setDetailError('')
    try {
      await apiClient.post(`/api/hr/kpi-reviews/${detail.id}/${action}`, payload ?? {})
      const { data } = await apiClient.get(`/api/hr/kpi-reviews/${detail.id}`)
      setDetail(data)
      setRejecting(null)
      load()
    } catch (err) {
      setDetailError(err.response?.data?.error ?? 'Gagal memproses penilaian')
    } finally {
      setBusy(false)
    }
  }

  const columns = [
    { key: 'period', label: 'Periode' },
    { key: 'employee_name', label: 'Karyawan', maxWidth: 220 },
    {
      key: 'total_score',
      label: 'Nilai',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (v) => Number(v.total_score).toFixed(2),
    },
    {
      key: 'rating',
      label: 'Rating',
      render: (v) => <span className={RATING_CLASS[v.rating] ?? ''}>{v.rating || '—'}</span>,
    },
    {
      key: 'status',
      label: 'Status',
      render: (v) => <span className={`badge ${STATUS_BADGE[v.status] ?? 'text-bg-secondary'}`}>{v.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (v) => (
        <button type="button" className="btn btn-sm btn-outline-primary" onClick={() => openDetail(v)}>
          <i className="bi bi-list-check me-1" />
          Rincian
        </button>
      ),
    },
  ]

  const readOnly = detail && detail.status !== 'DRAFT'

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Penilaian KPI</h2>
          <div className="text-secondary small">
            Nilai = pencapaian (realisasi/target) dikali bobot indikator. Pencapaian dibatasi 150% supaya satu
            indikator yang melonjak tidak menutupi indikator lain yang gagal.
          </div>
        </div>
        {can('create') && (
          <button
            type="button"
            className="btn btn-primary btn-sm"
            disabled={!companyId}
            onClick={() => {
              setCreateForm({ employee_id: '', period: currentPeriod() })
              setCreateError('')
              setCreating(true)
            }}
          >
            <i className="bi bi-plus-lg me-1" />
            Buat Penilaian
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {!loading && !companyId && (
        <div className="alert alert-warning py-2 small">Pilih company dulu untuk melihat penilaian.</div>
      )}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={reviews}
          loading={loading}
          searchPlaceholder="Cari karyawan atau periode..."
          emptyMessage="Belum ada penilaian KPI."
        />
      </div>

      {creating && (
        <Modal
          title="Buat Penilaian KPI"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="kpi-review-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Buat'}
              </button>
            </>
          }
        >
          <form id="kpi-review-form" onSubmit={handleCreate} className="d-flex flex-column gap-3">
            {createError && <div className="alert alert-danger py-2 small mb-0">{createError}</div>}
            <div>
              <label className="form-label">Karyawan</label>
              <select
                className="form-select"
                value={createForm.employee_id}
                onChange={(e) => setCreateForm({ ...createForm, employee_id: e.target.value })}
                required
              >
                <option value="">Pilih karyawan...</option>
                {employees.map((emp) => (
                  <option key={emp.id} value={emp.id}>
                    {emp.first_name} {emp.last_name} ({emp.employee_code})
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="form-label">Periode</label>
              <input
                type="month"
                className="form-control"
                value={createForm.period}
                onChange={(e) => setCreateForm({ ...createForm, period: e.target.value })}
                required
              />
              <div className="form-text">
                Seluruh indikator aktif akan disalin ke penilaian ini beserta target dan bobotnya.
              </div>
            </div>
          </form>
        </Modal>
      )}

      {detail && (
        <Modal
          title={`Penilaian ${detail.period} — ${detail.employee_name}`}
          onClose={() => setDetail(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setDetail(null)}>
                Tutup
              </button>
              {!readOnly && can('update') && (
                <button type="button" className="btn btn-outline-primary" onClick={saveScores} disabled={busy}>
                  {busy ? 'Menyimpan...' : 'Simpan Nilai'}
                </button>
              )}
              {detail.status === 'DRAFT' && can('update') && (
                <button type="button" className="btn btn-primary" onClick={() => runAction('submit')} disabled={busy}>
                  Ajukan
                </button>
              )}
              {detail.status === 'REJECTED' && can('update') && (
                <button type="button" className="btn btn-primary" onClick={() => runAction('submit')} disabled={busy}>
                  Ajukan Ulang
                </button>
              )}
              {detail.status === 'SUBMITTED' && can('approve') && (
                <>
                  <button
                    type="button"
                    className="btn btn-outline-danger"
                    onClick={() => {
                      setRejectionReason('')
                      setRejecting(detail)
                    }}
                    disabled={busy}
                  >
                    Tolak
                  </button>
                  <button type="button" className="btn btn-success" onClick={() => runAction('approve')} disabled={busy}>
                    Setujui
                  </button>
                </>
              )}
            </>
          }
        >
          <div className="d-flex flex-column gap-3">
            {detailError && <div className="alert alert-danger py-2 small mb-0">{detailError}</div>}
            {detail.rejection_reason && (
              <div className="alert alert-danger py-2 small mb-0">
                Ditolak: {detail.rejection_reason}
              </div>
            )}
            {readOnly && (
              <div className="alert alert-secondary py-2 small mb-0">
                Status {detail.status} &mdash; nilainya terkunci. Hanya penilaian DRAFT yang bisa diubah.
              </div>
            )}
            {Math.round(detail.total_weight * 100) / 100 !== 100 && (
              <div className="alert alert-warning py-2 small mb-0">
                Total bobot penilaian ini {Math.round(detail.total_weight * 100) / 100}% &mdash; harus tepat
                100% sebelum bisa diajukan.
              </div>
            )}

            <div className="table-responsive">
              <table className="table table-sm align-middle mb-0">
                <thead>
                  <tr>
                    <th>Indikator</th>
                    <th className="text-end">Target</th>
                    <th className="text-end" style={{ width: 140 }}>
                      Realisasi
                    </th>
                    <th className="text-end">Bobot</th>
                    <th className="text-end">Pencapaian</th>
                    <th className="text-end">Nilai</th>
                  </tr>
                </thead>
                <tbody>
                  {detail.items.map((it) => (
                    <tr key={it.indicator_id}>
                      <td>{it.indicator_name}</td>
                      <td className="text-end text-secondary">
                        {Number(it.target_value).toLocaleString('id-ID')} {it.unit}
                      </td>
                      <td className="text-end">
                        <input
                          type="number"
                          step="any"
                          min="0"
                          className="form-control form-control-sm text-end"
                          value={it.actual_value}
                          disabled={readOnly}
                          onChange={(e) => setItemValue(it.indicator_id, e.target.value)}
                        />
                      </td>
                      <td className="text-end text-secondary">{Number(it.weight)}%</td>
                      <td className="text-end">{Number(it.achievement).toFixed(2)}%</td>
                      <td className="text-end fw-semibold">{Number(it.score).toFixed(2)}</td>
                    </tr>
                  ))}
                </tbody>
                <tfoot>
                  <tr>
                    <td colSpan={5} className="text-end fw-semibold">
                      Total Nilai
                    </td>
                    <td className="text-end fw-bold">{Number(detail.total_score).toFixed(2)}</td>
                  </tr>
                  <tr>
                    <td colSpan={5} className="text-end fw-semibold">
                      Rating
                    </td>
                    <td className={`text-end ${RATING_CLASS[detail.rating] ?? ''}`}>{detail.rating || '—'}</td>
                  </tr>
                </tfoot>
              </table>
            </div>

            <div>
              <label className="form-label">Catatan</label>
              <input
                type="text"
                className="form-control"
                value={detail.notes ?? ''}
                disabled={readOnly}
                onChange={(e) => setDetail({ ...detail, notes: e.target.value })}
              />
            </div>
          </div>
        </Modal>
      )}

      {rejecting && (
        <Modal
          title="Tolak Penilaian"
          onClose={() => setRejecting(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setRejecting(null)}>
                Batal
              </button>
              <button
                type="button"
                className="btn btn-danger"
                disabled={busy || !rejectionReason.trim()}
                onClick={() => runAction('reject', { rejection_reason: rejectionReason })}
              >
                Tolak
              </button>
            </>
          }
        >
          <div>
            <label className="form-label">Alasan penolakan</label>
            <textarea
              className="form-control"
              rows={3}
              value={rejectionReason}
              onChange={(e) => setRejectionReason(e.target.value)}
              required
            />
            <div className="form-text">
              Penilaian yang ditolak kembali bisa diubah dan diajukan ulang, bukan hangus.
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

export default KpiReviewsPage
