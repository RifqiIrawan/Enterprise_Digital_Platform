import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import { useCompany } from '../../store/CompanyContext.jsx'

const STATUS_BADGE = {
  DRAFT: 'text-bg-secondary',
  IN_PROGRESS: 'text-bg-info',
  COMPLETED: 'text-bg-success',
  CANCELLED: 'text-bg-danger',
}

function ProductionSchedulePage() {
  const { companyId } = useCompany()
  const [orders, setOrders] = useState([])
  const [boms, setBoms] = useState([])
  const [products, setProducts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    setLoading(true)
    Promise.all([
      apiClient.get('/api/production/work-orders', { params: { company_id: companyId } }),
      apiClient.get('/api/production/boms', { params: { company_id: companyId } }),
      apiClient.get('/api/warehouse/products', { params: { company_id: companyId } }),
    ])
      .then(([ordersRes, bomsRes, productsRes]) => {
        setOrders(ordersRes.data)
        setBoms(bomsRes.data)
        setProducts(productsRes.data)
      })
      .catch(() => setError('Gagal memuat jadwal produksi. Pastikan production-service aktif.'))
      .finally(() => setLoading(false))
  }, [companyId])

  const bomName = (id) => boms.find((b) => b.id === id)?.name ?? id
  const productName = (id) => {
    const p = products.find((p) => p.id === id)
    return p ? `${p.sku} - ${p.name}` : id
  }

  const groups = orders
    .filter((o) => o.status !== 'CANCELLED')
    .reduce((acc, o) => {
      const key = o.planned_start_date
      acc[key] = acc[key] || []
      acc[key].push(o)
      return acc
    }, {})
  const dates = Object.keys(groups).sort()

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">Jadwal Produksi</h2>
        <div className="text-secondary small">Work order dikelompokkan berdasarkan tanggal rencana mulai.</div>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {loading && <div className="text-secondary small">Memuat...</div>}
      {!loading && !error && dates.length === 0 && (
        <div className="card p-4 text-center text-secondary">Belum ada work order terjadwal.</div>
      )}

      {dates.map((date) => (
        <div key={date} className="card p-3">
          <h6 className="mb-3">{new Date(date).toLocaleDateString('id-ID', { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' })}</h6>
          <div className="d-flex flex-column gap-2">
            {groups[date].map((o) => (
              <div key={o.id} className="d-flex align-items-center justify-content-between border rounded p-2">
                <div>
                  <div className="fw-semibold"><code>{o.wo_number}</code> &middot; {productName(o.product_id)}</div>
                  <div className="text-secondary small">
                    BOM: {bomName(o.bom_id)} &middot; Rencana {o.quantity_planned} pcs
                    {o.planned_end_date && ` → selesai ${new Date(o.planned_end_date).toLocaleDateString('id-ID')}`}
                  </div>
                </div>
                <span className={`badge ${STATUS_BADGE[o.status] ?? 'text-bg-secondary'}`}>{o.status}</span>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

export default ProductionSchedulePage
