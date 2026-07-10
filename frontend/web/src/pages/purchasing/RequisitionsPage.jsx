import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import Modal from '../../components/common/Modal.jsx'
import DataTable from '../../components/common/DataTable.jsx'

const emptyLine = { product_name: '', description: '', quantity: 1, estimated_price: '' }
const emptyForm = {
  requested_by: '',
  pr_date: new Date().toISOString().slice(0, 10),
  notes: '',
  lines: [{ ...emptyLine }],
}

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  SUBMITTED: 'text-bg-info',
  APPROVED: 'text-bg-success',
  REJECTED: 'text-bg-danger',
  CONVERTED: 'text-bg-primary',
}

function RequisitionsPage() {
  const [companyId, setCompanyId] = useState('')
  const [suppliers, setSuppliers] = useState([])
  const [requisitions, setRequisitions] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState(null)

  const [convertingPR, setConvertingPR] = useState(null)
  const [convertSupplierId, setConvertSupplierId] = useState('')
  const [convertError, setConvertError] = useState('')
  const [convertSaving, setConvertSaving] = useState(false)

  function loadRequisitions(cid) {
    setLoading(true)
    apiClient
      .get('/api/purchasing/requisitions', { params: { company_id: cid } })
      .then(({ data }) => setRequisitions(data))
      .catch(() => setError('Gagal memuat data purchase requisition. Pastikan purchasing-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) {
          loadRequisitions(cid)
          apiClient.get('/api/purchasing/suppliers', { params: { company_id: cid } }).then(({ data }) => setSuppliers(data))
        } else {
          setLoading(false)
        }
      })
      .catch(() => {
        setError('Gagal memuat data company.')
        setLoading(false)
      })
  }, [])

  function updateLine(index, patch) {
    setForm((f) => ({ ...f, lines: f.lines.map((l, i) => (i === index ? { ...l, ...patch } : l)) }))
  }

  function addLine() {
    setForm((f) => ({ ...f, lines: [...f.lines, { ...emptyLine }] }))
  }

  function removeLine(index) {
    setForm((f) => ({ ...f, lines: f.lines.filter((_, i) => i !== index) }))
  }

  const subtotal = form.lines.reduce((sum, l) => sum + (Number(l.quantity) || 0) * (Number(l.estimated_price) || 0), 0)

  async function handleSubmit(e) {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await apiClient.post('/api/purchasing/requisitions', {
        company_id: companyId,
        requested_by: form.requested_by,
        pr_date: form.pr_date,
        notes: form.notes,
        lines: form.lines
          .filter((l) => l.product_name)
          .map((l) => ({
            product_name: l.product_name,
            description: l.description,
            quantity: Number(l.quantity) || 0,
            estimated_price: Number(l.estimated_price) || 0,
          })),
      })
      setCreating(false)
      setForm(emptyForm)
      loadRequisitions(companyId)
    } catch (err) {
      setFormError(err.response?.data?.error ?? 'Gagal membuat purchase requisition')
    } finally {
      setSaving(false)
    }
  }

  async function handleAction(id, action) {
    setActingId(id)
    try {
      await apiClient.post(`/api/purchasing/requisitions/${id}/${action}`)
      loadRequisitions(companyId)
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal memproses purchase requisition')
    } finally {
      setActingId(null)
    }
  }

  function openConvert(pr) {
    setConvertingPR(pr)
    setConvertSupplierId('')
    setConvertError('')
  }

  async function handleConvert(e) {
    e.preventDefault()
    setConvertSaving(true)
    setConvertError('')
    try {
      await apiClient.post(`/api/purchasing/requisitions/${convertingPR.id}/convert`, { supplier_id: convertSupplierId })
      setConvertingPR(null)
      loadRequisitions(companyId)
    } catch (err) {
      setConvertError(err.response?.data?.error ?? 'Gagal mengkonversi requisition')
    } finally {
      setConvertSaving(false)
    }
  }

  const columns = [
    { key: 'pr_number', label: 'No. PR', render: (pr) => <code>{pr.pr_number}</code> },
    { key: 'requested_by', label: 'Pemohon' },
    {
      key: 'pr_date',
      label: 'Tanggal',
      cellClassName: 'text-secondary small',
      render: (pr) => new Date(pr.pr_date).toLocaleDateString('id-ID'),
    },
    {
      key: 'subtotal_amount',
      label: 'Estimasi',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (pr) => formatMoney(pr.subtotal_amount),
    },
    {
      key: 'status',
      label: 'Status',
      render: (pr) => <span className={`badge ${STATUS_BADGE[pr.status] ?? 'text-bg-secondary'}`}>{pr.status}</span>,
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (pr) => (
        <div className="d-flex gap-1 justify-content-end">
          {pr.status === 'DRAFT' && (
            <button type="button" className="btn btn-sm btn-outline-info" disabled={actingId === pr.id} onClick={() => handleAction(pr.id, 'submit')}>
              Ajukan
            </button>
          )}
          {pr.status === 'SUBMITTED' && (
            <>
              <button type="button" className="btn btn-sm btn-outline-success" disabled={actingId === pr.id} onClick={() => handleAction(pr.id, 'approve')}>
                Approve
              </button>
              <button type="button" className="btn btn-sm btn-outline-danger" disabled={actingId === pr.id} onClick={() => handleAction(pr.id, 'reject')}>
                Reject
              </button>
            </>
          )}
          {pr.status === 'APPROVED' && (
            <button type="button" className="btn btn-sm btn-outline-primary" onClick={() => openConvert(pr)}>
              Jadikan PO
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
          <h2 className="edp-page-title">Purchase Requisitions</h2>
          <div className="text-secondary small">Permintaan pembelian internal, dikonversi menjadi Purchase Order setelah disetujui.</div>
        </div>
        <button type="button" className="btn btn-primary btn-sm" disabled={!companyId} onClick={() => setCreating(true)}>
          <i className="bi bi-plus-lg me-1" />
          Buat Requisition
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={requisitions}
          loading={loading}
          searchPlaceholder="Cari no. requisition..."
          emptyMessage="Belum ada purchase requisition."
        />
      </div>

      {creating && (
        <Modal
          title="Buat Purchase Requisition"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreating(false)}>
                Batal
              </button>
              <button type="submit" form="pr-form" className="btn btn-primary" disabled={saving}>
                {saving ? 'Menyimpan...' : 'Simpan sebagai Draft'}
              </button>
            </>
          }
        >
          <form id="pr-form" onSubmit={handleSubmit} className="d-flex flex-column gap-3">
            {formError && <div className="alert alert-danger py-2 small mb-0">{formError}</div>}
            <div className="row g-3">
              <div className="col-6">
                <label className="form-label">Pemohon</label>
                <input
                  type="text"
                  className="form-control"
                  placeholder="mis. Divisi IT"
                  value={form.requested_by}
                  onChange={(e) => setForm({ ...form, requested_by: e.target.value })}
                />
              </div>
              <div className="col-6">
                <label className="form-label">Tanggal</label>
                <input
                  type="date"
                  className="form-control"
                  value={form.pr_date}
                  onChange={(e) => setForm({ ...form, pr_date: e.target.value })}
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

            <div>
              <div className="d-flex justify-content-between align-items-center mb-2">
                <label className="form-label mb-0">Baris Requisition</label>
                <button type="button" className="btn btn-sm btn-outline-secondary" onClick={addLine}>
                  <i className="bi bi-plus-lg me-1" />
                  Baris
                </button>
              </div>
              <div className="table-responsive">
                <table className="table table-sm align-middle mb-0">
                  <thead>
                    <tr>
                      <th>Produk / Jasa</th>
                      <th>Deskripsi</th>
                      <th style={{ width: 70 }}>Qty</th>
                      <th style={{ width: 130 }}>Estimasi Harga</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {form.lines.map((l, i) => (
                      <tr key={i}>
                        <td>
                          <input
                            type="text"
                            className="form-control form-control-sm"
                            value={l.product_name}
                            onChange={(e) => updateLine(i, { product_name: e.target.value })}
                          />
                        </td>
                        <td>
                          <input
                            type="text"
                            className="form-control form-control-sm"
                            value={l.description}
                            onChange={(e) => updateLine(i, { description: e.target.value })}
                          />
                        </td>
                        <td>
                          <input
                            type="number"
                            className="form-control form-control-sm"
                            value={l.quantity}
                            onChange={(e) => updateLine(i, { quantity: e.target.value })}
                            min="0"
                          />
                        </td>
                        <td>
                          <input
                            type="number"
                            className="form-control form-control-sm"
                            value={l.estimated_price}
                            onChange={(e) => updateLine(i, { estimated_price: e.target.value })}
                            min="0"
                          />
                        </td>
                        <td>
                          {form.lines.length > 1 && (
                            <button type="button" className="btn btn-sm btn-outline-danger" onClick={() => removeLine(i)}>
                              <i className="bi bi-x" />
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                  <tfoot>
                    <tr>
                      <td colSpan={3}></td>
                      <td className="fw-semibold text-nowrap">Total Estimasi</td>
                      <td className="fw-semibold">{formatMoney(subtotal)}</td>
                    </tr>
                  </tfoot>
                </table>
              </div>
            </div>
          </form>
        </Modal>
      )}

      {convertingPR && (
        <Modal
          title={`Jadikan PO: ${convertingPR.pr_number}`}
          onClose={() => setConvertingPR(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setConvertingPR(null)}>
                Batal
              </button>
              <button type="submit" form="convert-pr-form" className="btn btn-primary" disabled={convertSaving}>
                {convertSaving ? 'Memproses...' : 'Jadikan PO'}
              </button>
            </>
          }
        >
          <form id="convert-pr-form" onSubmit={handleConvert} className="d-flex flex-column gap-3">
            {convertError && <div className="alert alert-danger py-2 small mb-0">{convertError}</div>}
            <div className="text-secondary small">Pilih supplier untuk Purchase Order hasil konversi requisition ini.</div>
            <div>
              <label className="form-label">Supplier</label>
              <select
                className="form-select"
                value={convertSupplierId}
                onChange={(e) => setConvertSupplierId(e.target.value)}
                required
              >
                <option value="">Pilih supplier...</option>
                {suppliers.map((s) => (
                  <option key={s.id} value={s.id}>{s.supplier_code} - {s.name}</option>
                ))}
              </select>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default RequisitionsPage
