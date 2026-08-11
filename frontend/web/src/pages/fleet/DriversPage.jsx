import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { driver_code: '', name: '', phone: '', license_number: '', notes: '' }

const STATUS_BADGE = {
  AVAILABLE: 'text-bg-success',
  ON_DELIVERY: 'text-bg-warning',
  INACTIVE: 'text-bg-secondary',
}

// ON_DELIVERY sengaja TIDAK ada di dropdown -- digerakkan otomatis oleh surat
// jalan, sama seperti IN_USE di halaman Kendaraan.
const EDITABLE_STATUSES = ['AVAILABLE', 'INACTIVE']

function DriversPage() {
  const { companyId, branchId } = useCompany()
  const [drivers, setDrivers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [status, setStatus] = useState('AVAILABLE')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadDrivers(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/fleet/drivers', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setDrivers(data))
      .catch(() => setError('Gagal memuat data pengemudi. Pastikan fleet-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadDrivers(companyId, branchId)
  }, [companyId, branchId])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setStatus('AVAILABLE')
    setFormError('')
    setEditing(true)
  }

  function openEdit(d) {
    setEditingId(d.id)
    setForm({
      driver_code: d.driver_code,
      name: d.name,
      phone: d.phone ?? '',
      license_number: d.license_number ?? '',
      notes: d.notes ?? '',
    })
    setStatus(EDITABLE_STATUSES.includes(d.status) ? d.status : 'AVAILABLE')
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    const payload = {
      company_id: companyId,
      name: form.name,
      phone: form.phone,
      license_number: form.license_number,
      notes: form.notes,
    }
    try {
      if (editingId) {
        await apiClient.put(`/api/fleet/drivers/${editingId}`, { ...payload, status })
      } else {
        await apiClient.post('/api/fleet/drivers', {
          ...payload,
          branch_id: branchId || null,
          driver_code: form.driver_code,
        })
      }
      setEditing(false)
      loadDrivers(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data pengemudi')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'driver_code', label: 'Kode', render: (d) => <code>{d.driver_code}</code> },
    { key: 'name', label: 'Nama' },
    { key: 'phone', label: 'Telepon' },
    { key: 'license_number', label: 'No. SIM' },
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
          <h2 className="edp-page-title">Pengemudi</h2>
          <div className="text-secondary small">Pengemudi armada. Status ON_DELIVERY diatur otomatis oleh surat jalan.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Pengemudi
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={drivers} loading={loading} searchPlaceholder="Cari kode atau nama pengemudi..." emptyMessage="Belum ada pengemudi." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Pengemudi' : 'Tambah Pengemudi'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="driver-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="driver-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Pengemudi</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.driver_code}
                  onChange={(e) => setForm({ ...form, driver_code: e.target.value })}
                  disabled={!!editingId}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Nama</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>
              <div className="col-6">
                <label className="form-label">Telepon</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.phone}
                  onChange={(e) => setForm({ ...form, phone: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">No. SIM</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.license_number}
                  onChange={(e) => setForm({ ...form, license_number: e.target.value })}
                />
              </div>
              {editingId && (
                <div className="col-6">
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

export default DriversPage
