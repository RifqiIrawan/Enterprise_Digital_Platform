import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'

const emptyForm = {
  standard_id: '',
  reference_type: 'MANUAL',
  reference_id: '',
  reference_number: '',
  inspected_quantity: 1,
  passed_quantity: 1,
  failed_quantity: 0,
  inspection_date: new Date().toISOString().slice(0, 10),
  notes: '',
}

const RESULT_BADGE = {
  PASS: 'text-bg-success',
  FAIL: 'text-bg-danger',
  PARTIAL: 'text-bg-warning',
}

const REFERENCE_LABEL = {
  PURCHASE_ORDER: 'Purchase Order',
  WORK_ORDER: 'Work Order',
  MANUAL: 'Manual',
}

function QualityInspectionsPage() {
  const [companyId, setCompanyId] = useState('')
  const [standards, setStandards] = useState([])
  const [products, setProducts] = useState([])
  const [purchaseOrders, setPurchaseOrders] = useState([])
  const [workOrders, setWorkOrders] = useState([])
  const [inspections, setInspections] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function loadInspections(cid) {
    setLoading(true)
    apiClient
      .get('/api/qc/inspections', { params: { company_id: cid } })
      .then(({ data }) => setInspections(data))
      .catch(() => setError('Gagal memuat data inspeksi. Pastikan qc-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) {
          loadInspections(cid)
          apiClient.get('/api/qc/standards', { params: { company_id: cid } }).then(({ data }) => setStandards(data.filter((s) => s.is_active)))
          apiClient.get('/api/warehouse/products', { params: { company_id: cid } }).then(({ data }) => setProducts(data))
          apiClient.get('/api/purchasing/purchase-orders', { params: { company_id: cid } }).then(({ data }) => setPurchaseOrders(data))
          apiClient.get('/api/production/work-orders', { params: { company_id: cid } }).then(({ data }) => setWorkOrders(data))
        } else {
          setLoading(false)
        }
      })
      .catch(() => {
        setError('Gagal memuat data company.')
        setLoading(false)
      })
  }, [])

  const standardName = (id) => standards.find((s) => s.id === id)?.name ?? id
  const productName = (id) => {
    const p = products.find((p) => p.id === id)
    return p ? `${p.sku} - ${p.name}` : id
  }

  function openCreate() {
    setForm({ ...emptyForm })
    setFormError('')
    setCreating(true)
  }

  function updateReferenceType(type) {
    setForm({ ...form, reference_type: type, reference_id: '', reference_number: '' })
  }

  function updateReference(id, number) {
    setForm({ ...form, reference_id: id, reference_number: number })
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/qc/inspections', {
        company_id: companyId,
        standard_id: form.standard_id,
        reference_type: form.reference_type,
        reference_id: form.reference_id || undefined,
        reference_number: form.reference_number || undefined,
        inspected_quantity: Number(form.inspected_quantity) || 0,
        passed_quantity: Number(form.passed_quantity) || 0,
        failed_quantity: Number(form.failed_quantity) || 0,
        inspection_date: form.inspection_date,
        notes: form.notes,
      })
      setCreating(false)
      loadInspections(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat inspeksi')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    { key: 'inspection_number', label: 'No. Inspeksi', render: (i) => <code>{i.inspection_number}</code> },
    { key: 'product_id', label: 'Produk', render: (i) => productName(i.product_id), sortValue: (i) => productName(i.product_id) },
    { key: 'standard_id', label: 'Standar', render: (i) => standardName(i.standard_id), sortValue: (i) => standardName(i.standard_id) },
    {
      key: 'reference_type',
      label: 'Referensi',
      render: (i) => (
        <div>
          <div>{REFERENCE_LABEL[i.reference_type] ?? i.reference_type}</div>
          {i.reference_number && <div className="text-secondary small">{i.reference_number}</div>}
        </div>
      ),
    },
    { key: 'inspected_quantity', label: 'Diperiksa', className: 'text-end', cellClassName: 'text-end' },
    { key: 'passed_quantity', label: 'Lolos', className: 'text-end', cellClassName: 'text-end text-success' },
    { key: 'failed_quantity', label: 'Gagal', className: 'text-end', cellClassName: 'text-end text-danger' },
    {
      key: 'result',
      label: 'Hasil',
      render: (i) => <span className={`badge ${RESULT_BADGE[i.result] ?? 'text-bg-secondary'}`}>{i.result}</span>,
    },
    {
      key: 'inspection_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (i) => new Date(i.inspection_date).toLocaleDateString('id-ID'),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Inspeksi Kualitas</h2>
          <div className="text-secondary small">Catatan hasil pemeriksaan mutu, opsional terhubung ke Purchase Order atau Work Order.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId || standards.length === 0} onClick={openCreate}>
          <i className="bi bi-plus-lg me-1" />
          Catat Inspeksi
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {standards.length === 0 && !loading && !error && (
        <div className="alert alert-warning py-2 small mb-0">Belum ada standar mutu aktif. Buat standar dulu di menu Standar Mutu.</div>
      )}

      <div className="card p-3">
        <DataTable columns={columns} data={inspections} loading={loading} searchPlaceholder="Cari no. inspeksi..." emptyMessage="Belum ada inspeksi." />
      </div>

      {creating && (
        <Modal
          title="Catat Inspeksi Kualitas"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="inspection-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="inspection-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div>
              <label className="form-label">Standar Mutu</label>
              <select
                className="form-select"
                value={form.standard_id}
                onChange={(e) => setForm({ ...form, standard_id: e.target.value })}
                required
              >
                <option value="">Pilih standar...</option>
                {standards.map((s) => (
                  <option key={s.id} value={s.id}>{s.standard_code} - {s.name} ({productName(s.product_id)})</option>
                ))}
              </select>
            </div>

            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Referensi</label>
                <select className="form-select" value={form.reference_type} onChange={(e) => updateReferenceType(e.target.value)}>
                  <option value="MANUAL">Manual</option>
                  <option value="PURCHASE_ORDER">Purchase Order</option>
                  <option value="WORK_ORDER">Work Order</option>
                </select>
              </div>
              {form.reference_type === 'PURCHASE_ORDER' && (
                <div className="col-6">
                  <label className="form-label">No. PO</label>
                  <select
                    className="form-select"
                    value={form.reference_id}
                    onChange={(e) => {
                      const po = purchaseOrders.find((p) => p.id === e.target.value)
                      updateReference(e.target.value, po?.po_number ?? '')
                    }}
                  >
                    <option value="">Pilih PO...</option>
                    {purchaseOrders.map((po) => (
                      <option key={po.id} value={po.id}>{po.po_number}</option>
                    ))}
                  </select>
                </div>
              )}
              {form.reference_type === 'WORK_ORDER' && (
                <div className="col-6">
                  <label className="form-label">No. WO</label>
                  <select
                    className="form-select"
                    value={form.reference_id}
                    onChange={(e) => {
                      const wo = workOrders.find((w) => w.id === e.target.value)
                      updateReference(e.target.value, wo?.wo_number ?? '')
                    }}
                  >
                    <option value="">Pilih WO...</option>
                    {workOrders.map((wo) => (
                      <option key={wo.id} value={wo.id}>{wo.wo_number}</option>
                    ))}
                  </select>
                </div>
              )}
            </div>

            <div className="row g-3">
              <div className="col-4">
                <label className="form-label">Qty Diperiksa</label>
                <input
                  type="number"
                  className="form-control"
                  value={form.inspected_quantity}
                  onChange={(e) => setForm({ ...form, inspected_quantity: e.target.value })}
                  min="0"
                  required
                />
              </div>
              <div className="col-4">
                <label className="form-label">Qty Lolos</label>
                <input
                  type="number"
                  className="form-control"
                  value={form.passed_quantity}
                  onChange={(e) => setForm({ ...form, passed_quantity: e.target.value })}
                  min="0"
                />
              </div>
              <div className="col-4">
                <label className="form-label">Qty Gagal</label>
                <input
                  type="number"
                  className="form-control"
                  value={form.failed_quantity}
                  onChange={(e) => setForm({ ...form, failed_quantity: e.target.value })}
                  min="0"
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal Inspeksi</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.inspection_date}
                  onChange={(e) => setForm({ ...form, inspection_date: e.target.value })}
                  required
                />
              </div>
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

export default QualityInspectionsPage
