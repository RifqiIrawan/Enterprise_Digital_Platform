import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { vehicle_code: '', plate_number: '', name: '', capacity_kg: '', notes: '' }

const TYPE_LABEL = { MOTORCYCLE: 'Motor', VAN: 'Van', TRUCK: 'Truk' }

const STATUS_BADGE = {
  AVAILABLE: 'text-bg-success',
  IN_USE: 'text-bg-warning',
  MAINTENANCE: 'text-bg-secondary',
}

// IN_USE sengaja TIDAK ada di dropdown: status itu digerakkan otomatis oleh
// surat jalan (dispatch/deliver), backend menolak kalau dikirim manual.
const EDITABLE_STATUSES = ['AVAILABLE', 'MAINTENANCE']

function VehiclesPage() {
  const { companyId, branchId } = useCompany()
  const [vehicles, setVehicles] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [vehicleType, setVehicleType] = useState('VAN')
  const [status, setStatus] = useState('AVAILABLE')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadVehicles(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/fleet/vehicles', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setVehicles(data))
      .catch(() => setError('Gagal memuat data kendaraan. Pastikan fleet-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadVehicles(companyId, branchId)
  }, [companyId, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setVehicleType('VAN')
    setStatus('AVAILABLE')
    setFormError('')
    setEditing(true)
  }

  function openEdit(v) {
    setEditingId(v.id)
    setForm({
      vehicle_code: v.vehicle_code,
      plate_number: v.plate_number,
      name: v.name,
      capacity_kg: v.capacity_kg ?? '',
      notes: v.notes ?? '',
    })
    setVehicleType(v.vehicle_type)
    // Kendaraan yang sedang IN_USE tidak bisa diubah statusnya di sini
    // (backend menolak) -- dropdown jatuh ke AVAILABLE supaya tidak
    // menampilkan pilihan yang pasti gagal.
    setStatus(EDITABLE_STATUSES.includes(v.status) ? v.status : 'AVAILABLE')
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    const payload = {
      company_id: companyId,
      plate_number: form.plate_number,
      name: form.name,
      vehicle_type: vehicleType,
      capacity_kg: Number(form.capacity_kg) || 0,
      notes: form.notes,
    }
    try {
      if (editingId) {
        await apiClient.put(`/api/fleet/vehicles/${editingId}`, { ...payload, status })
      } else {
        await apiClient.post('/api/fleet/vehicles', {
          ...payload,
          branch_id: branchId || null,
          vehicle_code: form.vehicle_code,
        })
      }
      setEditing(false)
      loadVehicles(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data kendaraan')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'vehicle_code', label: 'Kode', render: (v) => <code>{v.vehicle_code}</code> },
    {
      key: 'name',
      label: 'Kendaraan',
      render: (v) => (
        <div>
          <div>{v.name}</div>
          <div className="text-secondary small">{v.plate_number}</div>
        </div>
      ),
    },
    { key: 'vehicle_type', label: 'Tipe', render: (v) => TYPE_LABEL[v.vehicle_type] ?? v.vehicle_type },
    {
      key: 'capacity_kg',
      label: 'Kapasitas',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (v) => `${new Intl.NumberFormat('id-ID').format(v.capacity_kg ?? 0)} kg`,
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
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(v)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Kendaraan</h2>
          <div className="text-secondary small">Armada pengiriman. Status IN_USE diatur otomatis oleh surat jalan.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Kendaraan
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={vehicles} loading={loading} searchPlaceholder="Cari kode, nama, atau plat nomor..." emptyMessage="Belum ada kendaraan." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Kendaraan' : 'Tambah Kendaraan'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="vehicle-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="vehicle-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Kendaraan</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.vehicle_code}
                  onChange={(e) => setForm({ ...form, vehicle_code: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Plat Nomor</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.plate_number}
                  onChange={(e) => setForm({ ...form, plate_number: e.target.value })}
                  required
                />
              </div>
              <div className="col-12">
                <label className="form-label">Nama / Model</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              <div className="col-4">
                <label className="form-label">Tipe</label>
                <select className="form-select" value={vehicleType} onChange={(e) => setVehicleType(e.target.value)}>
                  {Object.entries(TYPE_LABEL).map(([value, label]) => (
                    <option key={value} value={value}>{label}</option>
                  ))}
                </select>
              </div>
              <div className="col-4">
                <label className="form-label">Kapasitas (kg)</label>
                <input
                  type="number"
                  min="0"
                  step="0.01"
                  className="form-control"
                  value={form.capacity_kg}
                  onChange={(e) => setForm({ ...form, capacity_kg: e.target.value })}
                />
              </div>
              {editingId && (
                <div className="col-4">
                  <label className="form-label">Status</label>
                  <select className="form-select" value={status} onChange={(e) => setStatus(e.target.value)}>
                    {EDITABLE_STATUSES.map((s) => (
                      <option key={s} value={s}>{s}</option>
                    ))}
                  </select>
                </div>
              )}
              <div className="col-12">
                <label className="form-label">Catatan</label>
                <textarea
                  className="form-control"
                  rows={2}
                  value={form.notes}
                  onChange={(e) => setForm({ ...form, notes: e.target.value })}
                />
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default VehiclesPage
