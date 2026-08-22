import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const emptyForm = {
  vehicle_id: '',
  driver_id: '',
  ecommerce_order_id: '',
  recipient_name: '',
  recipient_phone: '',
  destination_address: '',
  scheduled_date: '',
  notes: '',
}

const STATUS_BADGE = {
  PENDING: 'text-bg-secondary',
  DISPATCHED: 'text-bg-info',
  DELIVERED: 'text-bg-success',
  CANCELLED: 'text-bg-danger',
}

// Transisi status hanya lewat endpoint khusus (tidak ada PUT status), jadi
// tombolnya dipetakan per status saat ini -- pola yang sama dengan
// OrdersPage e-commerce.
const STATUS_ACTIONS = {
  PENDING: [
    { action: 'dispatch', label: 'Berangkatkan', className: 'btn-outline-primary', icon: 'bi-send' },
    { action: 'cancel', label: 'Batalkan', className: 'btn-outline-danger', icon: 'bi-x-lg' },
  ],
  DISPATCHED: [
    { action: 'deliver', label: 'Selesai', className: 'btn-outline-success', icon: 'bi-check-lg' },
    { action: 'cancel', label: 'Batalkan', className: 'btn-outline-danger', icon: 'bi-x-lg' },
  ],
}

function DeliveryOrdersPage() {
  const { companyId, branchId } = useCompany()
  const { can } = usePagePermission()
  const [deliveries, setDeliveries] = useState([])
  const [vehicles, setVehicles] = useState([])
  const [drivers, setDrivers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionError, setActionError] = useState('')
  const [busyId, setBusyId] = useState(null)

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadDeliveries(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/fleet/delivery-orders', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setDeliveries(data))
      .catch(() => setError('Gagal memuat surat jalan. Pastikan fleet-service aktif.'))
      .finally(() => setLoading(false))
  }

  // Kendaraan & pengemudi dipakai untuk dropdown pemilihan di form.
  function loadAssignables(cid, bid) {
    apiClient
      .get('/api/fleet/vehicles', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setVehicles(data))
      .catch(() => setVehicles([]))
    apiClient
      .get('/api/fleet/drivers', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setDrivers(data))
      .catch(() => setDrivers([]))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadDeliveries(companyId, branchId)
    loadAssignables(companyId, branchId)
  }, [companyId, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(d) {
    setEditingId(d.id)
    setForm({
      vehicle_id: d.vehicle_id,
      driver_id: d.driver_id,
      ecommerce_order_id: d.ecommerce_order_id ?? '',
      recipient_name: d.recipient_name,
      recipient_phone: d.recipient_phone ?? '',
      destination_address: d.destination_address,
      scheduled_date: d.scheduled_date ? d.scheduled_date.slice(0, 10) : '',
      notes: d.notes ?? '',
    })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    const payload = {
      company_id: companyId,
      vehicle_id: form.vehicle_id,
      driver_id: form.driver_id,
      recipient_name: form.recipient_name,
      recipient_phone: form.recipient_phone,
      destination_address: form.destination_address,
      scheduled_date: form.scheduled_date,
      notes: form.notes,
    }
    try {
      if (editingId) {
        await apiClient.put(`/api/fleet/delivery-orders/${editingId}`, payload)
      } else {
        await apiClient.post('/api/fleet/delivery-orders', {
          ...payload,
          branch_id: branchId || null,
          // Kosongkan kalau tidak diisi -- backend memperlakukan string kosong
          // sebagai "pengiriman bukan dari order online".
          ecommerce_order_id: form.ecommerce_order_id || '',
        })
      }
      setEditing(false)
      loadDeliveries(companyId, branchId)
      loadAssignables(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan surat jalan')
    } finally {
      setSaving(false)
    }
  }

  async function runAction(delivery, action) {
    setActionError('')
    setBusyId(delivery.id)
    try {
      await apiClient.post(`/api/fleet/delivery-orders/${delivery.id}/${action}`)
      loadDeliveries(companyId, branchId)
      // Status kendaraan/pengemudi ikut berubah di server, jadi dropdown
      // dimuat ulang supaya tidak menampilkan status basi.
      loadAssignables(companyId, branchId)
    } catch (err) {
      setActionError(err.response?.data?.error ?? `Gagal menjalankan aksi ${action}`)
    } finally {
      setBusyId(null)
    }
  }

  const vehicleLabel = (id) => {
    const v = vehicles.find((x) => x.id === id)
    return v ? `${v.vehicle_code} — ${v.plate_number}` : '-'
  }
  const driverLabel = (id) => {
    const d = drivers.find((x) => x.id === id)
    return d ? d.name : '-'
  }

  const columns = [
    { key: 'delivery_number', label: 'No. Surat Jalan', render: (d) => <code>{d.delivery_number}</code> },
    {
      key: 'recipient_name',
      label: 'Penerima',
      render: (d) => (
        <div>
          <div>{d.recipient_name}</div>
          <div className="text-secondary small">{d.destination_address}</div>
        </div>
      ),
    },
    {
      key: 'reference_number',
      label: 'Order',
      render: (d) => (d.reference_number ? <code>{d.reference_number}</code> : <span className="text-secondary small">Internal</span>),
    },
    {
      key: 'vehicle_id',
      label: 'Armada',
      render: (d) => (
        <div className="small">
          {vehicleLabel(d.vehicle_id)}
          <br />
          <span className="text-secondary">{driverLabel(d.driver_id)}</span>
        </div>
      ),
    },
    {
      key: 'scheduled_date',
      label: 'Jadwal',
      render: (d) => (d.scheduled_date ? new Date(d.scheduled_date).toLocaleDateString('id-ID') : '-'),
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
        <div className="d-flex gap-1 justify-content-end">
          {d.status === 'PENDING' && can('update') && (
            <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(d)}>
              <i className="bi bi-pencil" />
            </button>
          )}
          {can('update') && (STATUS_ACTIONS[d.status] ?? []).map((a) => (
            <button
              key={a.action}
              type="button"
              className={`btn btn-sm ${a.className}`}
              disabled={busyId === d.id}
              onClick={() => runAction(d, a.action)}
              title={a.label}
            >
              <i className={`bi ${a.icon} me-1`} />
              {a.label}
            </button>
          ))}
        </div>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Surat Jalan</h2>
          <div className="text-secondary small">
            Penugasan pengiriman. Surat jalan yang terhubung ke order e-commerce otomatis menandai order itu DELIVERED saat diselesaikan.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
            <i className="bi bi-plus-lg me-1" />
            Buat Surat Jalan
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {actionError && <div className="alert alert-warning py-2 small">{actionError}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={deliveries} loading={loading} searchPlaceholder="Cari nomor surat jalan atau penerima..." emptyMessage="Belum ada surat jalan." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Surat Jalan' : 'Buat Surat Jalan'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="delivery-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="delivery-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kendaraan</label>
                <select
                  className="form-select"
                  value={form.vehicle_id}
                  onChange={(e) => setForm({ ...form, vehicle_id: e.target.value })}
                  required
                >
                  <option value="">-- pilih kendaraan --</option>
                  {vehicles.map((v) => (
                    <option key={v.id} value={v.id}>
                      {v.vehicle_code} — {v.plate_number} ({v.status})
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-6">
                <label className="form-label">Pengemudi</label>
                <select
                  className="form-select"
                  value={form.driver_id}
                  onChange={(e) => setForm({ ...form, driver_id: e.target.value })}
                  required
                >
                  <option value="">-- pilih pengemudi --</option>
                  {drivers.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.driver_code} — {d.name} ({d.status})
                    </option>
                  ))}
                </select>
              </div>
              {!editingId && (
                <div className="col-12">
                  <label className="form-label">ID Order E-Commerce (opsional)</label>
                  <input
                    type="text"
                    className="form-control"
                    placeholder="Kosongkan untuk pengiriman internal"
                    value={form.ecommerce_order_id}
                    onChange={(e) => setForm({ ...form, ecommerce_order_id: e.target.value })}
                  />
                  <div className="form-text">
                    Kalau diisi, order harus berstatus SHIPPED. Nama & alamat penerima akan diambil otomatis dari order kalau dikosongkan.
                  </div>
                </div>
              )}
              <div className="col-6">
                <label className="form-label">Nama Penerima</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.recipient_name}
                  onChange={(e) => setForm({ ...form, recipient_name: e.target.value })}
                  required={!!editingId || !form.ecommerce_order_id}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Telepon Penerima</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.recipient_phone}
                  onChange={(e) => setForm({ ...form, recipient_phone: e.target.value })}
                />
              </div>
              <div className="col-12">
                <label className="form-label">Alamat Tujuan</label>
                <textarea
                  className="form-control"
                  rows={2}
                  value={form.destination_address}
                  onChange={(e) => setForm({ ...form, destination_address: e.target.value })}
                  required={!!editingId || !form.ecommerce_order_id}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Jadwal</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.scheduled_date}
                  onChange={(e) => setForm({ ...form, scheduled_date: e.target.value })}
                />
              </div>
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

export default DeliveryOrdersPage
