import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'

const emptyForm = { code: '', name: '', address: '', is_active: true }

function WarehousesPage() {
  const [companyId, setCompanyId] = useState('')
  const [warehouses, setWarehouses] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadWarehouses(cid) {
    setLoading(true)
    apiClient
      .get('/api/warehouse/warehouses', { params: { company_id: cid } })
      .then(({ data }) => setWarehouses(data))
      .catch(() => setError('Gagal memuat data gudang. Pastikan warehouse-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) loadWarehouses(cid)
        else setLoading(false)
      })
      .catch(() => {
        setError('Gagal memuat data company.')
        setLoading(false)
      })
  }, [])

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(wh) {
    setEditingId(wh.id)
    setForm({ code: wh.code, name: wh.name, address: wh.address ?? '', is_active: wh.is_active })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/warehouse/warehouses/${editingId}`, {
          name: form.name,
          address: form.address,
          is_active: form.is_active,
        })
      } else {
        await apiClient.post('/api/warehouse/warehouses', {
          company_id: companyId,
          code: form.code,
          name: form.name,
          address: form.address,
        })
      }
      setEditing(false)
      loadWarehouses(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan data gudang')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'code', label: 'Kode', render: (wh) => <code>{wh.code}</code> },
    {
      key: 'name',
      label: 'Nama',
      render: (wh) => (
        <div>
          <div>{wh.name}</div>
          <div className="text-secondary small">{wh.address}</div>
        </div>
      ),
    },
    {
      key: 'is_active',
      label: 'Status',
      render: (wh) => <span className={`badge ${wh.is_active ? 'text-bg-success' : 'text-bg-secondary'}`}>{wh.is_active ? 'Aktif' : 'Nonaktif'}</span>,
      sortValue: (wh) => (wh.is_active ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (wh) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(wh)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Master Gudang</h2>
          <div className="text-secondary small">Daftar gudang/lokasi penyimpanan stok per branch.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Tambah Gudang
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={warehouses}
          loading={loading}
          searchPlaceholder="Cari kode atau nama gudang..."
          emptyMessage="Belum ada gudang. Tambahkan data gudang terlebih dahulu."
        />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Gudang' : 'Tambah Gudang'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="warehouse-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="warehouse-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Gudang</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.code}
                  onChange={(e) => setForm({ ...form, code: e.target.value })}
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
              <div className="col-12">
                <label className="form-label">Alamat</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.address}
                  onChange={(e) => setForm({ ...form, address: e.target.value })}
                />
              </div>
              {editingId && (
                <div className="col-6 d-flex align-items-end">
                  <div className="form-check">
                    <input
                      type="checkbox"
                      className="form-check-input"
                      id="warehouse-is-active"
                      checked={form.is_active}
                      onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
                    />
                    <label className="form-check-label" htmlFor="warehouse-is-active">Aktif</label>
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

export default WarehousesPage
