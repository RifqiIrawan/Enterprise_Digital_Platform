import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const DEVICE_TYPES = ['TEMPERATURE', 'HUMIDITY', 'VIBRATION', 'RFID', 'GPS', 'BARCODE']
const NUMERIC_TYPES = ['TEMPERATURE', 'HUMIDITY', 'VIBRATION']

const STATUS_BADGE = {
  ACTIVE: 'text-bg-success',
  INACTIVE: 'text-bg-secondary',
  MAINTENANCE: 'text-bg-warning',
}

const emptyForm = {
  device_code: '',
  device_type: 'TEMPERATURE',
  name: '',
  warehouse_id: '',
  status: 'ACTIVE',
  threshold_min: '',
  threshold_max: '',
}

function DevicesPage() {
  const { companyId, branchId } = useCompany()
  const [warehouses, setWarehouses] = useState([])
  const [devices, setDevices] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadDevices(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/iot/devices', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setDevices(data))
      .catch(() => setError('Gagal memuat data device. Pastikan iot-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadDevices(companyId, branchId)
    apiClient.get('/api/warehouse/warehouses', { params: { company_id: companyId } }).then(({ data }) => setWarehouses(data))
  }, [companyId, branchId])

  const warehouseName = (id) => {
    if (!id) return '—'
    const w = warehouses.find((w) => w.id === id)
    return w ? `${w.code} - ${w.name}` : id
  }

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(d) {
    setEditingId(d.id)
    setForm({
      device_code: d.device_code,
      device_type: d.device_type,
      name: d.name,
      warehouse_id: d.warehouse_id ?? '',
      status: d.status,
      threshold_min: d.threshold_min ?? '',
      threshold_max: d.threshold_max ?? '',
    })
    setFormError('')
    setEditing(true)
  }

  const isNumericType = NUMERIC_TYPES.includes(form.device_type)

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const thresholdMin = isNumericType && form.threshold_min !== '' ? Number(form.threshold_min) : null
      const thresholdMax = isNumericType && form.threshold_max !== '' ? Number(form.threshold_max) : null
      if (editingId) {
        await apiClient.put(`/api/iot/devices/${editingId}`, {
          warehouse_id: form.warehouse_id || null,
          name: form.name,
          status: form.status,
          threshold_min: thresholdMin,
          threshold_max: thresholdMax,
        })
      } else {
        await apiClient.post('/api/iot/devices', {
          company_id: companyId,
          branch_id: branchId || null,
          warehouse_id: form.warehouse_id || null,
          device_code: form.device_code,
          device_type: form.device_type,
          name: form.name,
          threshold_min: thresholdMin,
          threshold_max: thresholdMax,
        })
      }
      setEditing(false)
      loadDevices(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan device')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'device_code', label: 'Kode', render: (d) => <code>{d.device_code}</code> },
    { key: 'name', label: 'Nama' },
    { key: 'device_type', label: 'Tipe' },
    { key: 'warehouse_id', label: 'Lokasi Gudang', render: (d) => warehouseName(d.warehouse_id), sortValue: (d) => warehouseName(d.warehouse_id) },
    {
      key: 'threshold',
      label: 'Threshold',
      sortable: false,
      render: (d) => (d.threshold_min != null && d.threshold_max != null ? `${d.threshold_min} – ${d.threshold_max}` : '—'),
    },
    {
      key: 'status',
      label: 'Status',
      render: (d) => <span className={`badge ${STATUS_BADGE[d.status] ?? 'text-bg-secondary'}`}>{d.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (d) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(d)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">IoT Devices</h2>
          <div className="text-secondary small">Pendataan sensor/alat simulasi (temperature, humidity, vibration, RFID, GPS, barcode).</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Device
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={devices} loading={loading} searchPlaceholder="Cari kode atau nama device..." emptyMessage="Belum ada device." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Device' : 'Tambah Device'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="device-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="device-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Device</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.device_code}
                  onChange={(e) => setForm({ ...form, device_code: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tipe Device</label>
                <select
                  className="form-select"
                  value={form.device_type}
                  onChange={(e) => setForm({ ...form, device_type: e.target.value, threshold_min: '', threshold_max: '' })}
                  disabled={!!editingId}
                  required
                >
                  {DEVICE_TYPES.map((t) => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>
              <div className="col-12">
                <label className="form-label">Nama</label>
                <input type="text" className="form-control" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
              </div>
              <div className="col-12">
                <label className="form-label">Lokasi Gudang (opsional)</label>
                <select className="form-select" value={form.warehouse_id} onChange={(e) => setForm({ ...form, warehouse_id: e.target.value })}>
                  <option value="">Tidak ditempatkan di gudang tertentu</option>
                  {warehouses.map((w) => (
                    <option key={w.id} value={w.id}>{w.code} - {w.name}</option>
                  ))}
                </select>
              </div>
              {isNumericType && (
                <>
                  <div className="col-6">
                    <label className="form-label">Threshold Minimum</label>
                    <input
                      type="number"
                      step="any"
                      className="form-control"
                      value={form.threshold_min}
                      onChange={(e) => setForm({ ...form, threshold_min: e.target.value })}
                      placeholder="Kosongkan kalau tidak perlu alert"
                    />
                  </div>
                  <div className="col-6">
                    <label className="form-label">Threshold Maksimum</label>
                    <input
                      type="number"
                      step="any"
                      className="form-control"
                      value={form.threshold_max}
                      onChange={(e) => setForm({ ...form, threshold_max: e.target.value })}
                      placeholder="Kosongkan kalau tidak perlu alert"
                    />
                  </div>
                </>
              )}
              {editingId && (
                <div className="col-12">
                  <label className="form-label">Status</label>
                  <select className="form-select" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })} required>
                    <option value="ACTIVE">ACTIVE</option>
                    <option value="INACTIVE">INACTIVE</option>
                    <option value="MAINTENANCE">MAINTENANCE</option>
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

export default DevicesPage
