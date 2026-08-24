import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { code: '', name: '', start_time: '08:00', end_time: '16:00', break_minutes: 60, is_active: true }

function formatMinutes(total) {
  const hours = Math.floor(total / 60)
  const minutes = total % 60
  return minutes === 0 ? `${hours} jam` : `${hours} jam ${minutes} menit`
}

function ShiftsPage() {
  const { companyId } = useCompany()
  const { can } = usePagePermission()
  const [shifts, setShifts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(null) // null | 'new' | shift
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadShifts(cid) {
    setLoading(true)
    apiClient
      .get('/api/production/shifts', { params: { company_id: cid } })
      .then(({ data }) => setShifts(data))
      .catch(() => setError('Gagal memuat data shift. Pastikan production-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadShifts(companyId)
  }, [companyId])

  function openCreate() {
    setForm({ ...emptyForm })
    setFormError('')
    setEditing('new')
  }

  function openEdit(s) {
    setForm({
      code: s.code,
      name: s.name,
      start_time: s.start_time,
      end_time: s.end_time,
      break_minutes: s.break_minutes,
      is_active: s.is_active,
    })
    setFormError('')
    setEditing(s)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const payload = {
        name: form.name,
        start_time: form.start_time,
        end_time: form.end_time,
        break_minutes: Number(form.break_minutes) || 0,
        is_active: form.is_active,
      }
      if (editing === 'new') {
        await apiClient.post('/api/production/shifts', { ...payload, company_id: companyId, code: form.code })
      } else {
        await apiClient.put(`/api/production/shifts/${editing.id}`, payload)
      }
      setEditing(null)
      loadShifts(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan shift')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'code', label: 'Kode', render: (s) => <code>{s.code}</code> },
    { key: 'name', label: 'Nama Shift' },
    { key: 'start_time', label: 'Jam', render: (s) => `${s.start_time} – ${s.end_time}`, sortValue: (s) => s.start_time },
    { key: 'break_minutes', label: 'Istirahat', className: 'text-end', cellClassName: 'text-end', render: (s) => `${s.break_minutes} menit` },
    {
      key: 'planned_minutes',
      label: 'Waktu Produktif',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (s) => formatMinutes(s.planned_minutes),
      sortValue: (s) => s.planned_minutes,
    },
    {
      key: 'is_active',
      label: 'Status',
      render: (s) => <span className={`badge ${s.is_active ? 'text-bg-success' : 'text-bg-secondary'}`}>{s.is_active ? 'Aktif' : 'Nonaktif'}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (s) =>
        can('update') && (
          <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(s)}>
            Ubah
          </button>
        ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Shift Produksi</h2>
          <div className="text-secondary small">
            Waktu produktif shift (jam kerja dikurangi istirahat) menjadi waktu rencana setiap eksekusi produksi &mdash; penyebut faktor Availability di OEE.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Shift
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={shifts} loading={loading} searchPlaceholder="Cari kode atau nama shift..." emptyMessage="Belum ada shift." />
      </div>

      {editing && (
        <Modal
          title={editing === 'new' ? 'Tambah Shift' : `Ubah ${editing.code}`}
          onClose={() => setEditing(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(null)}>
                Batal
              </button>
              <button type="submit" form="shift-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="shift-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.code}
                  onChange={(e) => setForm({ ...form, code: e.target.value })}
                  disabled={editing !== 'new'}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Nama Shift</label>
                <input type="text" className="form-control" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
              </div>
              <div className="col-4">
                <label className="form-label">Mulai</label>
                <input type="time" className="form-control" value={form.start_time} onChange={(e) => setForm({ ...form, start_time: e.target.value })} required />
              </div>
              <div className="col-4">
                <label className="form-label">Selesai</label>
                <input type="time" className="form-control" value={form.end_time} onChange={(e) => setForm({ ...form, end_time: e.target.value })} required />
              </div>
              <div className="col-4">
                <label className="form-label">Istirahat (menit)</label>
                <input
                  type="number"
                  min="0"
                  className="form-control"
                  value={form.break_minutes}
                  onChange={(e) => setForm({ ...form, break_minutes: e.target.value })}
                  required
                />
              </div>
              <div className="col-12">
                <div className="text-secondary small">Shift yang melewati tengah malam (mis. 22:00 – 06:00) didukung; durasinya dihitung menyeberang hari.</div>
              </div>
              {editing !== 'new' && (
                <div className="col-12">
                  <div className="form-check">
                    <input
                      className="form-check-input"
                      type="checkbox"
                      id="shift-active"
                      checked={form.is_active}
                      onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
                    />
                    <label className="form-check-label" htmlFor="shift-active">
                      Shift aktif
                    </label>
                  </div>
                  <div className="form-text">
                    Mengubah jam shift tidak mengubah eksekusi produksi yang sudah tercatat &mdash; waktu rencananya disimpan saat run dibuat.
                  </div>
                </div>
              )}
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default ShiftsPage
