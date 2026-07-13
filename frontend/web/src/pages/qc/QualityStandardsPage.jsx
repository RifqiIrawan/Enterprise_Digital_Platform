import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const emptyForm = { standard_code: '', name: '', product_id: '', criteria: '' }

function QualityStandardsPage() {
  const { companyId, branchId } = useCompany()
  const [products, setProducts] = useState([])
  const [standards, setStandards] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [editing, setEditing] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadStandards(cid, bid) {
    setLoading(true)
    apiClient
      .get('/api/qc/standards', { params: { company_id: cid, branch_id: bid } })
      .then(({ data }) => setStandards(data))
      .catch(() => setError('Gagal memuat data standar mutu. Pastikan qc-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadStandards(companyId, branchId)
    apiClient.get('/api/warehouse/products', { params: { company_id: companyId } }).then(({ data }) => setProducts(data))
  }, [companyId, branchId])

  const productName = (id) => {
    const p = products.find((p) => p.id === id)
    return p ? `${p.sku} - ${p.name}` : id
  }

  function openCreate() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditing(true)
  }

  function openEdit(s) {
    setEditingId(s.id)
    setForm({ standard_code: s.standard_code, name: s.name, product_id: s.product_id, criteria: s.criteria ?? '' })
    setFormError('')
    setEditing(true)
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editingId) {
        await apiClient.put(`/api/qc/standards/${editingId}`, {
          name: form.name,
          criteria: form.criteria,
          is_active: true,
        })
      } else {
        await apiClient.post('/api/qc/standards', {
          company_id: companyId,
          branch_id: branchId || null,
          standard_code: form.standard_code,
          name: form.name,
          product_id: form.product_id,
          criteria: form.criteria,
        })
      }
      setEditing(false)
      loadStandards(companyId, branchId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal menyimpan standar mutu')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'standard_code', label: 'Kode', render: (s) => <code>{s.standard_code}</code> },
    { key: 'name', label: 'Nama' },
    { key: 'product_id', label: 'Produk', render: (s) => productName(s.product_id), sortValue: (s) => productName(s.product_id) },
    { key: 'criteria', label: 'Kriteria', cellClassName: 'text-secondary small', maxWidth: 260 },
    {
      key: 'is_active',
      label: 'Status',
      render: (s) => <span className={`badge ${s.is_active ? 'text-bg-success' : 'text-bg-secondary'}`}>{s.is_active ? 'Aktif' : 'Nonaktif'}</span>,
      sortValue: (s) => (s.is_active ? 1 : 0),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (s) => (
        <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEdit(s)}>
          <i className="bi bi-pencil" />
        </button>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Standar Mutu</h2>
          <div className="text-secondary small">Kriteria pass/fail per produk yang jadi acuan inspeksi kualitas.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Buat Standar
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable columns={columns} data={standards} loading={loading} searchPlaceholder="Cari kode atau nama standar..." emptyMessage="Belum ada standar mutu." />
      </div>

      {editing && (
        <Modal
          title={editingId ? 'Edit Standar Mutu' : 'Buat Standar Mutu'}
          onClose={() => setEditing(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditing(false)}>
                Batal
              </button>
              <button type="submit" form="standard-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="standard-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Kode Standar</label>
                <input
                  type="text"
                  className="form-control"
                  value={form.standard_code}
                  onChange={(e) => setForm({ ...form, standard_code: e.target.value })}
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
                <label className="form-label">Produk</label>
                <select
                  className="form-select"
                  value={form.product_id}
                  onChange={(e) => setForm({ ...form, product_id: e.target.value })}
                  disabled={!!editingId}
                  required
                >
                  <option value="">Pilih produk...</option>
                  {products.map((p) => (
                    <option key={p.id} value={p.id}>{p.sku} - {p.name}</option>
                  ))}
                </select>
              </div>
              <div className="col-12">
                <label className="form-label">Kriteria</label>
                <textarea
                  className="form-control"
                  rows={3}
                  value={form.criteria}
                  onChange={(e) => setForm({ ...form, criteria: e.target.value })}
                  placeholder="Deskripsi kriteria pass/fail, mis. toleransi ukuran, kondisi kemasan, dst."
                />
              </div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default QualityStandardsPage
