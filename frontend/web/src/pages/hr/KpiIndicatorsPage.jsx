import { useEffect, useMemo, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { code: '', name: '', description: '', unit: 'poin', target_value: '', weight: '', is_active: true }

const UNITS = ['poin', '%', 'unit', 'rupiah', 'jam', 'hari']

function KpiIndicatorsPage() {
  const { companyId } = useCompany()
  const { can } = usePagePermission()
  const [indicators, setIndicators] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
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
      .get('/api/hr/kpi-indicators', { params: { company_id: companyId } })
      .then(({ data }) => setIndicators(data))
      .catch(() => setError('Gagal memuat indikator KPI. Pastikan hr-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(load, [companyId])

  // Bobot indikator aktif harus tepat 100% -- penilaian tidak bisa diajukan
  // kalau tidak genap, jadi angkanya ditampilkan terus-menerus di sini.
  const activeWeight = useMemo(
    () => indicators.filter((i) => i.is_active).reduce((sum, i) => sum + Number(i.weight), 0),
    [indicators]
  )

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(i) {
    setEditingId(i.id)
    setForm({
      code: i.code,
      name: i.name,
      description: i.description ?? '',
      unit: i.unit,
      target_value: i.target_value,
      weight: i.weight,
      is_active: i.is_active,
    })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    const payload = {
      name: form.name,
      description: form.description,
      unit: form.unit,
      target_value: Number(form.target_value),
      weight: Number(form.weight),
      is_active: form.is_active,
    }
    try {
      if (editingId) {
        await apiClient.put(`/api/hr/kpi-indicators/${editingId}`, payload)
      } else {
        await apiClient.post('/api/hr/kpi-indicators', {
          company_id: companyId,
          code: form.code,
          ...payload,
        })
      }
      setEditing(false)
      load()
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan indikator')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(indicator) {
    if (!window.confirm(`Hapus indikator "${indicator.name}"?`)) return
    try {
      await apiClient.delete(`/api/hr/kpi-indicators/${indicator.id}`)
      load()
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal menghapus indikator')
    }
  }

  const columns = [
    { key: 'code', label: 'Code', render: (i) => <code>{i.code}</code> },
    { key: 'name', label: 'Indikator', maxWidth: 260 },
    {
      key: 'target_value',
      label: 'Target',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (i) => `${Number(i.target_value).toLocaleString('id-ID')} ${i.unit}`,
    },
    {
      key: 'weight',
      label: 'Bobot',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (i) => `${Number(i.weight)}%`,
    },
    {
      key: 'is_active',
      label: 'Status',
      render: (i) =>
        i.is_active ? (
          <span className="badge text-bg-success">Aktif</span>
        ) : (
          <span className="badge text-bg-secondary">Nonaktif</span>
        ),
      sortValue: (i) => (i.is_active ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (i) => (
        <div className="d-flex gap-1 justify-content-end">
          {can('update') && (
            <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(i)}>
              <i className="bi bi-pencil" />
            </button>
          )}
          {can('delete') && (
            <button type="button" className="btn btn-sm btn-outline-danger" onClick={() => handleDelete(i)}>
              <i className="bi bi-trash" />
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
          <h2 className="edp-page-title">Indikator KPI</h2>
          <div className="text-secondary small">
            Master indikator penilaian kinerja. Penilaian baru menyalin indikator yang aktif beserta target
            dan bobotnya &mdash; mengubah master di sini tidak mengubah penilaian yang sudah dibuat.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Indikator
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {!loading && !companyId && (
        <div className="alert alert-warning py-2 small">Pilih company dulu untuk melihat indikatornya.</div>
      )}
      {!loading && indicators.length > 0 && Math.round(activeWeight * 100) / 100 !== 100 && (
        <div className="alert alert-warning py-2 small mb-0">
          Total bobot indikator aktif <strong>{Math.round(activeWeight * 100) / 100}%</strong>, belum genap
          100% &mdash; penilaian tidak bisa diajukan sampai genap.
        </div>
      )}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={indicators}
          loading={loading}
          searchPlaceholder="Cari indikator..."
          emptyMessage="Belum ada indikator KPI."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Indikator' : 'Tambah Indikator'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="indicator-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="indicator-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            {!editingId && (
              <div>
                <label className="form-label">Code</label>
                <input
                  type="text"
                  className="form-control"
                  placeholder="mis. OMZET_BULANAN"
                  value={form.code}
                  onChange={(e) => setForm({ ...form, code: e.target.value })}
                  required
                />
                <div className="form-text">Otomatis jadi huruf besar; spasi diganti underscore.</div>
              </div>
            )}
            <div>
              <label className="form-label">Nama Indikator</label>
              <input
                type="text"
                className="form-control"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                required
              />
            </div>
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Target</label>
                <input
                  type="number"
                  step="any"
                  min="0.01"
                  className="form-control"
                  value={form.target_value}
                  onChange={(e) => setForm({ ...form, target_value: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Satuan</label>
                <select
                  className="form-select"
                  value={form.unit}
                  onChange={(e) => setForm({ ...form, unit: e.target.value })}
                >
                  {UNITS.map((u) => (
                    <option key={u} value={u}>
                      {u}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            <div>
              <label className="form-label">Bobot (%)</label>
              <input
                type="number"
                step="any"
                min="0.01"
                max="100"
                className="form-control"
                value={form.weight}
                onChange={(e) => setForm({ ...form, weight: e.target.value })}
                required
              />
              <div className="form-text">Jumlah bobot seluruh indikator aktif harus tepat 100%.</div>
            </div>
            <div>
              <label className="form-label">Keterangan</label>
              <input
                type="text"
                className="form-control"
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
              />
            </div>
            {editingId && (
              <div className="form-check">
                <input
                  id="indicator-active"
                  type="checkbox"
                  className="form-check-input"
                  checked={form.is_active}
                  onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
                />
                <label className="form-check-label" htmlFor="indicator-active">
                  Aktif
                </label>
                <div className="form-text">
                  Indikator yang sudah dipakai penilaian tidak bisa dihapus &mdash; nonaktifkan saja supaya
                  penilaian lama tetap utuh.
                </div>
              </div>
            )}
          </form>
        </Modal>
      )}
    </div>
  )
}

export default KpiIndicatorsPage
