import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import { useCompany } from '../../store/CompanyContext.jsx'
import GroupedBarChart from './GroupedBarChart.jsx'

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)
}

function formatQty(n) {
  return new Intl.NumberFormat('id-ID', { maximumFractionDigits: 0 }).format(n ?? 0)
}

// Dua dari tiga chart bulanan di halaman ini sama-sama dikotomi "masuk vs
// keluar" -- slot 1 (biru) selalu sisi masuk (Revenue, Stock In), slot 2
// (oranye) selalu sisi keluar (Expense, Stock Out). Urutan tetap ini SENGAJA
// dipakai ulang (bukan warna baru per chart) supaya pembaca dashboard
// belajar polanya sekali: biru = masuk, oranye = keluar, di seluruh halaman
// ini. Sales value bukan dikotomi (cuma satu ukuran magnitude, bukan
// dua sisi berlawanan) jadi cuma satu seri -- tetap slot 1 biru yang sama,
// bukan warna baru, karena tidak ada sisi "keluar" untuk dikontraskan.
const FINANCE_SERIES = [
  { key: 'revenue', label: 'Revenue', color: 'var(--bs-primary)' },
  { key: 'expense', label: 'Expense', color: 'var(--bs-orange)' },
]
const STOCK_SERIES = [
  { key: 'stock_in', label: 'Stock In', color: 'var(--bs-primary)' },
  { key: 'stock_out', label: 'Stock Out', color: 'var(--bs-orange)' },
]
const SALES_SERIES = [{ key: 'sales_value', label: 'Sales Value', color: 'var(--bs-primary)' }]

// Pipeline CRM SENGAJA tidak memakai pasangan biru/oranye di atas. Weighted
// value bukan lawan dari total value, tapi BAGIAN dari total itu (total
// dikalikan probability tiap deal) -- memberinya warna kategorikal kedua akan
// terbaca sebagai dua hal yang berlawanan, persis makna yang sudah dipakai
// biru/oranye di dua chart pertama. Satu hue yang sama dengan opacity lebih
// rendah adalah encoding yang jujur untuk "himpunan bagian dari bar
// sebelahnya", dan tidak mengotori konvensi masuk-vs-keluar halaman ini.
const CRM_PIPELINE_SERIES = [
  { key: 'total_amount', label: 'Nilai Total', color: 'var(--bs-primary)' },
  { key: 'weighted_amount', label: 'Nilai Terbobot (× probability)', color: 'rgba(var(--bs-primary-rgb), 0.4)' },
]

// Biaya proyek: satu ukuran magnitude (rupiah yang sudah masuk GL), bukan
// dikotomi masuk-vs-keluar dan bukan subset dari bar sebelahnya -- jadi satu
// seri, slot 1 biru yang sama seperti Sales Value. Jam kerja SENGAJA tidak
// dijadikan seri kedua di chart ini: satuannya beda (jam vs rupiah), dan dua
// satuan pada satu sumbu Y membuat tinggi bar-nya tidak bisa dibandingkan.
// Angkanya tetap tersedia lewat tooltip di kolom lain kalau nanti dibutuhkan.
const PROJECT_COST_SERIES = [{ key: 'posted_amount', label: 'Biaya Diposting', color: 'var(--bs-primary)' }]

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
  const { companyId } = useCompany()
  const [summary, setSummary] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [monthlyFinance, setMonthlyFinance] = useState(null)
  const [monthlyFinanceError, setMonthlyFinanceError] = useState('')
  const [monthlyStock, setMonthlyStock] = useState(null)
  const [monthlyStockError, setMonthlyStockError] = useState('')
  const [monthlySales, setMonthlySales] = useState(null)
  const [monthlySalesError, setMonthlySalesError] = useState('')
  const [crmPipeline, setCrmPipeline] = useState(null)
  const [crmPipelineError, setCrmPipelineError] = useState('')
  const [projectCost, setProjectCost] = useState(null)
  const [projectCostError, setProjectCostError] = useState('')

  function loadSummary(cid) {
    setLoading(true)
    apiClient
      .get('/api/ai-bi/dashboards/summary', { params: { company_id: cid } })
      .then(({ data }) => setSummary(data))
      .catch(() => setError('Gagal memuat dashboard. Pastikan ai-bi-service aktif.'))
      .finally(() => setLoading(false))
  }

  // Chart ini sumbernya dw-service (ClickHouse), bukan ai-bi-service seperti
  // stat tile lain di atas -- loading/error dipisah sendiri supaya dw-service
  // yang down tidak ikut menggagalkan bagian dashboard lain (pola toleransi
  // kegagalan sebagian yang sama dengan `summary.errors` di atas).
  function loadMonthlyFinance(cid) {
    setMonthlyFinanceError('')
    apiClient
      .get('/api/dw/analytics/finance-monthly-summary', { params: { company_id: cid } })
      .then(({ data }) => setMonthlyFinance(data.map((d) => ({ ...d, revenue: Number(d.revenue), expense: Number(d.expense) }))))
      .catch(() => setMonthlyFinanceError('Gagal memuat ringkasan finance bulanan. Pastikan dw-service aktif.'))
  }

  // Chart kedua dari dw-service, pola loading/error terpisah yang sama
  // dengan loadMonthlyFinance di atas.
  function loadMonthlyStock(cid) {
    setMonthlyStockError('')
    apiClient
      .get('/api/dw/analytics/stock-movement-monthly-summary', { params: { company_id: cid } })
      .then(({ data }) => setMonthlyStock(data.map((d) => ({ ...d, stock_in: Number(d.stock_in), stock_out: Number(d.stock_out) }))))
      .catch(() => setMonthlyStockError('Gagal memuat ringkasan pergerakan stok bulanan. Pastikan dw-service aktif.'))
  }

  // Chart ketiga dari dw-service, pola loading/error terpisah yang sama.
  function loadMonthlySales(cid) {
    setMonthlySalesError('')
    apiClient
      .get('/api/dw/analytics/sales-monthly-summary', { params: { company_id: cid } })
      .then(({ data }) => setMonthlySales(data.map((d) => ({ ...d, sales_value: Number(d.sales_value) }))))
      .catch(() => setMonthlySalesError('Gagal memuat ringkasan sales bulanan. Pastikan dw-service aktif.'))
  }

  // Chart keempat dari dw-service, dan yang pertama BUKAN time series bulanan
  // -- sumbu X-nya stage pipeline, urutannya sudah ditentukan backend (urutan
  // funnel, bukan alfabetis) jadi frontend cukup merender apa adanya.
  function loadCrmPipeline(cid) {
    setCrmPipelineError('')
    apiClient
      .get('/api/dw/analytics/crm-pipeline-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setCrmPipeline(
          data.map((d) => ({ ...d, total_amount: Number(d.total_amount), weighted_amount: Number(d.weighted_amount) })),
        ),
      )
      .catch(() => setCrmPipelineError('Gagal memuat pipeline CRM. Pastikan dw-service aktif.'))
  }

  // Chart kelima dari dw-service, sumbu X-nya kode proyek (kategorikal seperti
  // pipeline CRM). Backend sudah mengurutkan dari biaya terbesar, jadi frontend
  // merender apa adanya. Hanya timesheet POSTED yang dihitung di sana -- yang
  // tampil di sini adalah biaya yang benar-benar sudah masuk jurnal GL.
  function loadProjectCost(cid) {
    setProjectCostError('')
    apiClient
      .get('/api/dw/analytics/project-cost-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setProjectCost(
          data.map((d) => ({ ...d, posted_amount: Number(d.posted_amount), posted_hours: Number(d.posted_hours) })),
        ),
      )
      .catch(() => setProjectCostError('Gagal memuat biaya proyek. Pastikan dw-service aktif.'))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    loadSummary(companyId)
    loadMonthlyFinance(companyId)
    loadMonthlyStock(companyId)
    loadMonthlySales(companyId)
    loadCrmPipeline(companyId)
    loadProjectCost(companyId)
  }, [companyId])

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

          <div className="row g-3">
            <div className="col-md-4">
              <div className="card p-3 h-100">
                <h6 className="mb-3">Revenue vs Expense per Bulan (dari Data Warehouse)</h6>
                {monthlyFinanceError && <div className="alert alert-warning py-2 small mb-0">{monthlyFinanceError}</div>}
                {!monthlyFinanceError && monthlyFinance == null && <div className="text-secondary small">Memuat...</div>}
                {!monthlyFinanceError && monthlyFinance != null && (
                  <GroupedBarChart data={monthlyFinance} series={FINANCE_SERIES} formatValue={(v) => `Rp ${formatMoney(v)}`} />
                )}
              </div>
            </div>
            <div className="col-md-4">
              <div className="card p-3 h-100">
                <h6 className="mb-3">Stok Masuk vs Keluar per Bulan (dari Data Warehouse)</h6>
                {monthlyStockError && <div className="alert alert-warning py-2 small mb-0">{monthlyStockError}</div>}
                {!monthlyStockError && monthlyStock == null && <div className="text-secondary small">Memuat...</div>}
                {!monthlyStockError && monthlyStock != null && (
                  <GroupedBarChart data={monthlyStock} series={STOCK_SERIES} formatValue={(v) => formatQty(v)} />
                )}
              </div>
            </div>
            <div className="col-md-4">
              <div className="card p-3 h-100">
                <h6 className="mb-3">Sales Value per Bulan (dari Data Warehouse)</h6>
                {monthlySalesError && <div className="alert alert-warning py-2 small mb-0">{monthlySalesError}</div>}
                {!monthlySalesError && monthlySales == null && <div className="text-secondary small">Memuat...</div>}
                {!monthlySalesError && monthlySales != null && (
                  <GroupedBarChart data={monthlySales} series={SALES_SERIES} formatValue={(v) => `Rp ${formatMoney(v)}`} />
                )}
              </div>
            </div>
          </div>

          <div className="row g-3">
            <div className="col-md-8">
              <div className="card p-3 h-100">
                <h6 className="mb-3">Pipeline CRM per Stage (dari Data Warehouse)</h6>
                {crmPipelineError && <div className="alert alert-warning py-2 small mb-0">{crmPipelineError}</div>}
                {!crmPipelineError && crmPipeline == null && <div className="text-secondary small">Memuat...</div>}
                {!crmPipelineError && crmPipeline != null && (
                  <GroupedBarChart
                    data={crmPipeline}
                    series={CRM_PIPELINE_SERIES}
                    formatValue={(v) => `Rp ${formatMoney(v)}`}
                    categoryKey="stage"
                    formatCategoryTick={(v) => v}
                  />
                )}
              </div>
            </div>
          </div>

          <div className="row g-3">
            <div className="col-md-8">
              <div className="card p-3 h-100">
                <h6 className="mb-3">Biaya Proyek yang Sudah Diposting ke GL (dari Data Warehouse)</h6>
                {projectCostError && <div className="alert alert-warning py-2 small mb-0">{projectCostError}</div>}
                {!projectCostError && projectCost == null && <div className="text-secondary small">Memuat...</div>}
                {!projectCostError && projectCost != null && projectCost.length === 0 && (
                  <div className="text-secondary small">
                    Belum ada timesheet yang diposting ke GL. Biaya baru muncul di sini setelah timesheet disetujui dan
                    diposting dari halaman Proyek.
                  </div>
                )}
                {!projectCostError && projectCost != null && projectCost.length > 0 && (
                  <GroupedBarChart
                    data={projectCost}
                    series={PROJECT_COST_SERIES}
                    formatValue={(v) => `Rp ${formatMoney(v)}`}
                    categoryKey="project_code"
                    formatCategoryTick={(v) => v}
                  />
                )}
              </div>
            </div>
          </div>

          <div className="text-secondary small">Terakhir dimuat: {new Date(summary.generated_at).toLocaleString('id-ID')}</div>
        </>
      )}
    </div>
  )
}

export default BIDashboardsPage
