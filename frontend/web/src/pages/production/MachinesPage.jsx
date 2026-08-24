import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = { code: '', name: '', machine_type: 'GENERAL', location: '', ideal_cycle_time_minutes: 1, status: 'ACTIVE', notes: '' }

const STATUS_BADGE = {
  ACTIVE: 'text-bg-success',
  MAINTENANCE: 'text-bg-warning',
  INACTIVE: 'text-bg-secondary',
}

const MACHINE_TYPES = ['GENERAL', 'CUTTING', 'MOLDING', 'ASSEMBLY', 'PACKING', 'PRINTING', 'MIXING']

function MachinesPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [machines, setMachines] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(null) // null | 'new' | machine
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadMachines(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/production/machines', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setMachines(data))
      .catch(() => setError('Gagal memuat data mesin. Pastikan production-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadMachines(companyId, branchId)
  }, [companyId, branchId])

  function openCreate() {
    setForm({ ...emptyForm })
    setFormError('')
    setEditing('new')
  }

  function openEdit(m) {
    setForm({
      code: m.code,
      name: m.name,
      machine_type: m.machine_type,
      location: m.location ?? '',
      ideal_cycle_time_minutes: m.ideal_cycle_time_minutes,
      status: m.status,
      notes: m.notes ?? '',
    })
    setFormError('')
    setEditing(m)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const payload = {
        name: form.name,
        machine_type: form.machine_type,
        location: form.location,
        ideal_cycle_time_minutes: Number(form.ideal_cycle_time_minutes) || 0,
        notes: form.notes,
      }
      if (editing === 'new') {
        await apiClient.post('/api/production/machines', {
          ...payload,
          company_id: companyId,
          branch_id: branchId || null,
          code: form.code,
        })
      } else {
        await apiClient.put(`/api/production/machines/${editing.id}`, { ...payload, status: form.status })
      }
      setEditing(null)
      loadMachines(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan mesin')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'code', label: 'Kode', render: (m) => <code>{m.code}</code> },
    { key: 'name', label: 'Nama Mesin' },
    { key: 'machine_type', label: 'Tipe', cellClassName: 'text-secondary small' },
    { key: 'location', label: 'Lokasi', render: (m) => m.location || '—', sortValue: (m) => m.location ?? '' },
    {
      key: 'ideal_cycle_time_minutes',
      label: 'Cycle Time Ideal',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (m) => `${Number(m.ideal_cycle_time_minutes)} menit/unit`,
      sortValue: (m) => Number(m.ideal_cycle_time_minutes),
    },
    {
      key: 'status',
      label: 'Status',
      render: (m) => <span className={`badge ${STATUS_BADGE[m.status] ?? 'text-bg-secondary'}`}>{m.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (m) =>
        can('update') && (
          <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(m)}>
            Ubah
          </button>
        ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Mesin</h2>
          <div className="text-secondary small">
            Cycle time ideal adalah menit yang seharusnya dibutuhkan mesin untuk 1 unit &mdash; angka inilah penyebut faktor Performance di OEE.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Tambah Mesin
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={machines} loading={loading} searchPlaceholder="Cari kode atau nama mesin..." emptyMessage="Belum ada mesin." />
      </div>

      {editing && (
        <Modal
          title={editing === 'new' ? 'Tambah Mesin' : `Ubah ${editing.code}`}
          onClose={() => setEditing(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(null)}>
                Batal
              </button>
              <button type="submit" form="machine-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="machine-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
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
                <label className="form-label">Nama Mesin</label>
                <input type="text" className="form-control" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
              </div>
              <div className="col-6">
                <label className="form-label">Tipe</label>
                <select className="form-select" value={form.machine_type} onChange={(e) => setForm({ ...form, machine_type: e.target.value })}>
                  {MACHINE_TYPES.map((t) => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Lokasi</label>
                <input type="text" className="form-control" value={form.location} onChange={(e) => setForm({ ...form, location: e.target.value })} />
              </div>
              <div className="col-6">
                <label className="form-label">Cycle Time Ideal (menit/unit)</label>
                <input
                  type="number"
                  step="0.0001"
                  min="0"
                  className="form-control"
                  value={form.ideal_cycle_time_minutes}
                  onChange={(e) => setForm({ ...form, ideal_cycle_time_minutes: e.target.value })}
                  required
                />
              </div>
              {editing !== 'new' && (
                <div className="col-6">
                  <label className="form-label">Status</label>
                  <select className="form-select" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
                    <option value="ACTIVE">ACTIVE</option>
                    <option value="MAINTENANCE">MAINTENANCE</option>
                    <option value="INACTIVE">INACTIVE</option>
                  </select>
                  <div className="form-text">Mesin yang masih punya eksekusi produksi berjalan tidak bisa ditarik dari status ACTIVE.</div>
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

export default MachinesPage
