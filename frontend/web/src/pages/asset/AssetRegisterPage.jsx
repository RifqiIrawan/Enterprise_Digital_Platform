import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { asset_code: '', name: '', category: '', warehouse_id: '', acquisition_date: '', acquisition_cost: '', notes: '' }

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

const STATUS_BADGE = {
  ACTIVE: 'text-bg-success',
  MAINTENANCE: 'text-bg-warning',
  DISPOSED: 'text-bg-secondary',
}

function AssetRegisterPage() {
  const { companyId, branchId } = useCompany()
  const [warehouses, setWarehouses] = useState([])
  const [assets, setAssets] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [status, setStatus] = useState('ACTIVE')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadAssets(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/asset/assets', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setAssets(data))
      .catch(() => setError('Gagal memuat data aset. Pastikan asset-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadAssets(companyId, branchId)
    apiClient.get('/api/warehouse/warehouses', { params: { company_id: companyId } }).then(({ data }) => setWarehouses(data))
  }, [companyId, branchId])

  const warehouseName = (id) => {
    if (!id) return '—'
    return warehouses.find((w) => w.id === id)?.name ?? id
  }

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setStatus('ACTIVE')
    setFormError('')
    setEditing(true)
  }

  function openEdit(a) {
    setEditingId(a.id)
    setForm({
      asset_code: a.asset_code,
      name: a.name,
      category: a.category ?? '',
      warehouse_id: a.warehouse_id ?? '',
      acquisition_date: a.acquisition_date ? a.acquisition_date.slice(0, 10) : '',
      acquisition_cost: a.acquisition_cost ?? '',
      notes: a.notes ?? '',
    })
    setStatus(a.status)
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/asset/assets/${editingId}`, {
          warehouse_id: form.warehouse_id || null,
          name: form.name,
          category: form.category,
          status,
          notes: form.notes,
        })
      } else {
        await apiClient.post('/api/asset/assets', {
          company_id: companyId,
          branch_id: branchId || null,
          warehouse_id: form.warehouse_id || null,
          asset_code: form.asset_code,
          name: form.name,
          category: form.category,
          acquisition_date: form.acquisition_date || null,
          acquisition_cost: Number(form.acquisition_cost) || 0,
          notes: form.notes,
        })
      }
      setEditing(false)
      loadAssets(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data aset')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'asset_code', label: 'Kode', render: (a) => <code>{a.asset_code}</code> },
    {
      key: 'name',
      label: 'Nama',
      render: (a) => (
        <div>
          <div>{a.name}</div>
          <div className="text-secondary small">{a.category}</div>
        </div>
      ),
    },
    { key: 'warehouse_id', label: 'Lokasi', render: (a) => warehouseName(a.warehouse_id), sortValue: (a) => warehouseName(a.warehouse_id) },
    {
      key: 'acquisition_cost',
      label: 'Harga Perolehan',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (a) => formatMoney(a.acquisition_cost),
    },
    {
      key: 'status',
      label: 'Status',
      render: (a) => <span className={`badge ${STATUS_BADGE[a.status] ?? 'text-bg-secondary'}`}>{a.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (a) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(a)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Pendataan Aset</h2>
          <div className="text-secondary small">Daftar aset fisik perusahaan (mesin, kendaraan, peralatan, dst).</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Aset
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={assets} loading={loading} searchPlaceholder="Cari kode atau nama aset..." emptyMessage="Belum ada aset." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Aset' : 'Tambah Aset'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="asset-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="asset-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Aset</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.asset_code}
                  onChange={(e) => setForm({ ...form, asset_code: e.target.value })}
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
                <label className="form-label">Kategori</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.category}
                  onChange={(e) => setForm({ ...form, category: e.target.value })}
                  placeholder="Mesin, Kendaraan, Elektronik, ..."
                />
              </div>
              <div className="col-6">
                <label className="form-label">Lokasi Gudang (opsional)</label>
                <select
                  className="form-select"
                  value={form.warehouse_id}
                  onChange={(e) => setForm({ ...form, warehouse_id: e.target.value })}
                >
                  <option value="">Tidak ditentukan</option>
                  {warehouses.map((wh) => (
                    <option key={wh.id} value={wh.id}>{wh.code} - {wh.name}</option>
                  ))}
                </select>
              </div>
              {!editingId && (
                <>
                  <div className="col-6">
                    <label className="form-label">Tanggal Perolehan</label>
                    <input
                      type="date"
                      className="form-control"
                      value={form.acquisition_date}
                      onChange={(e) => setForm({ ...form, acquisition_date: e.target.value })}
                    />
                  </div>
                  <div className="col-6">
                    <label className="form-label">Harga Perolehan</label>
                    <input
                      type="number"
                      className="form-control"
                      value={form.acquisition_cost}
                      onChange={(e) => setForm({ ...form, acquisition_cost: e.target.value })}
                      min="0"
                    />
                  </div>
                </>
              )}
              {editingId && (
                <div className="col-6">
                  <label className="form-label">Status</label>
                  <select className="form-select" value={status} onChange={(e) => setStatus(e.target.value)}>
                    <option value="ACTIVE">Aktif</option>
                    <option value="MAINTENANCE">Maintenance</option>
                    <option value="DISPOSED">Dilepas/Dijual</option>
                  </select>
                </div>
              )}
              <div className="col-12">
                <label className="form-label">Catatan</label>
                <input
                  type="text"
                  className="form-control"
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

export default AssetRegisterPage
