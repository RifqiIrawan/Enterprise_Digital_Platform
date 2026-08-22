import { useEffect, useMemo, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { holiday_date: '', name: '', is_national: true }

const DAY_NAMES = ['Minggu', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu']

function formatDate(value) {
  const d = new Date(value)
  return `${DAY_NAMES[d.getDay()]}, ${d.toLocaleDateString('id-ID', { day: 'numeric', month: 'long', year: 'numeric' })}`
}

function HolidaysPage() {
  const { companyId } = useCompany()
  const { can } = usePagePermission()
  const currentYear = new Date().getFullYear()
  const [year, setYear] = useState(currentYear)
  const [holidays, setHolidays] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function load() {
    if (!companyId) {
      setLoading(false)
      return
    }
    setLoading(true)
    apiClient
      .get('/api/hr/holidays', { params: { company_id: companyId, year } })
      .then(({ data }) => setHolidays(data))
      .catch(() => setError('Gagal memuat kalender hari libur. Pastikan hr-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(load, [companyId, year])

  const yearOptions = useMemo(
    () => [currentYear - 1, currentYear, currentYear + 1],
    [currentYear]
  )

  function openCreate() {
    setForm({ ...emptyForm, holiday_date: `${year}-01-01` })
    setFormError('')
    setCreating(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/hr/holidays', { company_id: companyId, ...form })
      setCreating(false)
      load()
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan hari libur')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(holiday) {
    if (!window.confirm(`Hapus "${holiday.name}" dari kalender?`)) return
    try {
      await apiClient.delete(`/api/hr/holidays/${holiday.id}`)
      load()
    } catch {
      window.alert('Gagal menghapus hari libur')
    }
  }

  const columns = [
    { key: 'holiday_date', label: 'Tanggal', render: (h) => formatDate(h.holiday_date) },
    { key: 'name', label: 'Keterangan' },
    {
      key: 'is_national',
      label: 'Jenis',
      render: (h) =>
        h.is_national ? (
          <span className="badge text-bg-danger">Nasional</span>
        ) : (
          <span className="badge text-bg-secondary">Internal</span>
        ),
      sortValue: (h) => (h.is_national ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (h) =>
        can('delete') && (
          <button type="button" className="btn btn-sm btn-outline-danger" onClick={() => handleDelete(h)}>
            <i className="bi bi-trash" />
          </button>
        ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Kalender Hari Libur</h2>
          <div className="text-secondary small">
            Tanggal di sini bukan hari kerja: tidak dihitung sebagai hari cuti, tidak menjadi pembagi
            pro-rata payroll, dan membuat lembur di tanggal tersebut memakai tarif hari libur.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Hari Libur
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {!loading && !companyId && (
        <div className="alert alert-warning py-2 small">Pilih company dulu untuk melihat kalendernya.</div>
      )}

      <div className="card p-3">
        <div className="d-flex align-items-center gap-2 mb-3">
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
          <span className="text-body-tertiary small">{holidays.length} hari libur</span>
        </div>

        <DataTable
          columns={columns}
          data={holidays}
          loading={loading}
          searchPlaceholder="Cari keterangan..."
          emptyMessage="Belum ada hari libur di tahun ini."
        />
      </div>

      {creating && (
        <Modal
          title="Tambah Hari Libur"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="holiday-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="holiday-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div>
              <label className="form-label">Tanggal</label>
              <input
                type="date"
                className="form-control"
                value={form.holiday_date}
                onChange={(e) => setForm({ ...form, holiday_date: e.target.value })}
                required
              />
            </div>
            <div>
              <label className="form-label">Keterangan</label>
              <input
                type="text"
                className="form-control"
                placeholder="mis. Hari Kemerdekaan RI"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                required
              />
            </div>
            <div className="form-check">
              <input
                id="is-national"
                type="checkbox"
                className="form-check-input"
                checked={form.is_national}
                onChange={(e) => setForm({ ...form, is_national: e.target.checked })}
              />
              <label className="form-check-label" htmlFor="is-national">
                Tanggal merah nasional
              </label>
              <div className="form-text">
                Hilangkan centang untuk libur khusus perusahaan atau cuti bersama internal. Keduanya
                sama-sama dihitung sebagai bukan hari kerja &mdash; bedanya hanya penandaan.
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default HolidaysPage
