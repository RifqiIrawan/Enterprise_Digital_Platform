import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

function StatTile({ label, value, sub, className = '' }) {
  return (
    <div className="col-md-3 col-sm-6">
      <div className="card p-3 h-100">
        <div className="text-secondary small">{label}</div>
        <div className={`fs-3 fw-semibold ${className}`}>{value}</div>
        {sub && <div className="text-secondary small">{sub}</div>}
      </div>
    </div>
  )
}

// Warna status disamakan dengan STATUS_BADGE yang sudah dipakai di halaman
// Sales/Purchasing/Production Orders masing-masing, supaya konsisten dengan
// palet yang sudah ada di seluruh aplikasi (bukan palet baru untuk satu
// halaman dashboard ini saja).
const SALES_STATUS_COLOR = { DRAFT: 'bg-secondary', CONFIRMED: 'bg-info', FULFILLED: 'bg-warning', INVOICED: 'bg-success', CANCELLED: 'bg-danger' }
const PURCHASING_STATUS_COLOR = { DRAFT: 'bg-secondary', CONFIRMED: 'bg-info', RECEIVED: 'bg-warning', INVOICED: 'bg-success', CANCELLED: 'bg-danger' }
const PRODUCTION_STATUS_COLOR = { DRAFT: 'bg-secondary', IN_PROGRESS: 'bg-info', COMPLETED: 'bg-success', CANCELLED: 'bg-danger' }

function StatusBreakdown({ title, byStatus, colorMap }) {
  const entries = Object.entries(byStatus ?? {}).filter(([, count]) => count > 0)
  const max = Math.max(1, ...entries.map(([, count]) => count))
  return (
    <div className="card p-3">
      <h6 className="mb-3">{title}</h6>
      {entries.length === 0 && <div className="text-secondary small">Belum ada data.</div>}
      <div className="d-flex flex-column gap-2">
        {entries.map(([status, count]) => (
          <div key={status} className="d-flex align-items-center gap-2">
            <div className="text-secondary small" style={{ width: 110, flexShrink: 0 }}>{status}</div>
            <div className="flex-grow-1 bg-body-secondary rounded" style={{ height: 20 }}>
              <div
                className={`${colorMap[status] ?? 'bg-secondary'} rounded`}
                style={{ height: '100%', width: `${(count / max) * 100}%`, minWidth: 4 }}
              />
            </div>
            <div className="fw-semibold small" style={{ width: 24, textAlign: 'right' }}>{count}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function passRateColor(pct) {
  if (pct >= 90) return 'text-success'
  if (pct >= 70) return 'text-warning'
  return 'text-danger'
}

function BIDashboardsPage() {
  const [companyId, setCompanyId] = useState('')
  const [summary, setSummary] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  function loadSummary(cid) {
    setLoading(true)
    apiClient
      .get('/api/ai-bi/dashboards/summary', { params: { company_id: cid } })
      .then(({ data }) => setSummary(data))
      .catch(() => setError('Gagal memuat dashboard. Pastikan ai-bi-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) loadSummary(cid)
        else setLoading(false)
      })
      .catch(() => {
        setError('Gagal memuat data company.')
        setLoading(false)
      })
  }, [])

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">BI Dashboards</h2>
          <div className="text-secondary small">Ringkasan lintas modul, diambil langsung dari tiap service secara real-time.</div>
        </div>
        <button type="button" className="btn btn-outline-secondary btn-sm" disabled={!companyId || loading} onClick={() => loadSummary(companyId)}>
          <i className="bi bi-arrow-clockwise me-1" />
          Muat Ulang
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {loading && <div className="text-secondary small">Memuat...</div>}

      {summary && (
        <>
          {summary.errors?.length > 0 && (
            <div className="alert alert-warning py-2 small mb-0">
              Sebagian data gagal dimuat: {summary.errors.map((e) => e.source).join(', ')}. Bagian terkait ditampilkan sebagai 0.
            </div>
          )}

          <div className="row g-3">
            <StatTile label="Sales — Total Order" value={summary.sales.total_orders} sub={`Rp ${formatMoney(summary.sales.total_revenue)}`} />
            <StatTile label="Purchasing — Total Order" value={summary.purchasing.total_orders} sub={`Rp ${formatMoney(summary.purchasing.total_spend)}`} />
            <StatTile label="AR Outstanding" value={`Rp ${formatMoney(summary.finance.ar_outstanding)}`} sub={`dari Rp ${formatMoney(summary.finance.ar_total)}`} />
            <StatTile label="AP Outstanding" value={`Rp ${formatMoney(summary.finance.ap_outstanding)}`} sub={`dari Rp ${formatMoney(summary.finance.ap_total)}`} />
          </div>

          <div className="row g-3">
            <StatTile label="Produk / Gudang" value={summary.warehouse.total_products} sub={`${summary.warehouse.total_warehouses} gudang`} />
            <StatTile
              label="Stok Menipis"
              value={summary.warehouse.low_stock_count}
              sub={`dari ${summary.warehouse.total_stock_lines} baris stok`}
              className={summary.warehouse.low_stock_count > 0 ? 'text-warning' : ''}
            />
            <StatTile label="Karyawan Aktif" value={summary.hr.active_employees} sub={`dari ${summary.hr.total_employees} total`} />
            <StatTile
              label="Maintenance Overdue"
              value={summary.asset.overdue_maintenance_count}
              sub={`${summary.asset.total_assets} aset terdaftar`}
              className={summary.asset.overdue_maintenance_count > 0 ? 'text-danger' : ''}
            />
          </div>

          <div className="row g-3">
            <StatTile label="Jurnal GL" value={summary.finance.journal_entries_count} sub="entri" />
            <StatTile label="Work Order" value={summary.production.total_work_orders} sub="total" />
            <StatTile
              label="QC Pass Rate"
              value={`${summary.qc.pass_rate_pct.toFixed(1)}%`}
              sub={`${summary.qc.pass_count} pass / ${summary.qc.fail_count} fail / ${summary.qc.partial_count} partial`}
              className={passRateColor(summary.qc.pass_rate_pct)}
            />
          </div>

          <div className="row g-3">
            <div className="col-md-4">
              <StatusBreakdown title="Sales Order per Status" byStatus={summary.sales.by_status} colorMap={SALES_STATUS_COLOR} />
            </div>
            <div className="col-md-4">
              <StatusBreakdown title="Purchase Order per Status" byStatus={summary.purchasing.by_status} colorMap={PURCHASING_STATUS_COLOR} />
            </div>
            <div className="col-md-4">
              <StatusBreakdown title="Work Order per Status" byStatus={summary.production.by_status} colorMap={PRODUCTION_STATUS_COLOR} />
            </div>
          </div>

          <div className="text-secondary small">Terakhir dimuat: {new Date(summary.generated_at).toLocaleString('id-ID')}</div>
        </>
      )}
    </div>
  )
}

export default BIDashboardsPage
