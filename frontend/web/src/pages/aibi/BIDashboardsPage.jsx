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

// Rata-rata lama pengiriman dilaporkan untuk bulan TERAKHIR yang benar-benar
// punya pengiriman selesai, bukan dirata-ratakan lagi lintas bulan: merata-rata
// sekumpulan rata-rata tanpa membobot jumlah pengirimannya akan menghasilkan
// angka yang salah, dan membobotnya di frontend berarti menghitung ulang sesuatu
// yang seharusnya jadi tugas query.
function describeAvgDeliveryHours(rows) {
  const withAvg = rows.filter((r) => r.avg_delivery_hours != null)
  if (withAvg.length === 0) return 'Belum ada pengiriman selesai yang bisa dihitung durasinya.'
  const last = withAvg[withAvg.length - 1]
  const hours = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 1 }).format(last.avg_delivery_hours)
  return `Rata-rata lama pengiriman ${last.month.slice(0, 7)}: ${hours} jam (berangkat sampai tiba).`
}

function describeAttendance(rows) {
  const withPct = rows.filter((r) => r.attendance_pct != null)
  if (withPct.length === 0) return 'Belum ada payroll yang diposting ke GL.'
  const last = withPct[withPct.length - 1]
  const pct = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 1 }).format(last.attendance_pct)
  // Dihitung dari total hari, bukan rata-rata persentase per orang.
  return `Kehadiran ${last.period}: ${pct}% dari total hari kerja seluruh karyawan.`
}

function describeMaintenanceDelay(rows) {
  const withDelay = rows.filter((r) => r.avg_delay_days != null)
  const overdue = rows.reduce((sum, r) => sum + Number(r.overdue_count), 0)
  const overdueText = overdue > 0 ? `${overdue} jadwal masih terlambat. ` : ''
  if (withDelay.length === 0) return `${overdueText}Belum ada perawatan selesai yang bisa dihitung keterlambatannya.`
  const last = withDelay[withDelay.length - 1]
  const days = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 1 }).format(last.avg_delay_days)
  return `${overdueText}Selisih rata-rata ${last.month.slice(0, 7)}: ${days} hari dari jadwal.`
}

function describeAvgOrderValue(rows) {
  const withAvg = rows.filter((r) => r.avg_order_value != null)
  if (withAvg.length === 0) return 'Belum ada order yang tercatat.'
  const last = withAvg[withAvg.length - 1]
  return `Rata-rata nilai per order ${last.month.slice(0, 7)}: Rp ${formatMoney(last.avg_order_value)}.`
}

function describeDefectRate(rows) {
  const withRate = rows.filter((r) => r.defect_rate_pct != null)
  if (withRate.length === 0) return 'Belum ada inspeksi yang bisa dihitung tingkat cacatnya.'
  const last = withRate[withRate.length - 1]
  const pct = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 2 }).format(last.defect_rate_pct)
  // Dihitung dari kuantitas, bukan dari cacah inspeksi -- perbedaannya besar
  // kalau satu inspeksi mencakup ribuan unit.
  return `Tingkat cacat ${last.month.slice(0, 7)}: ${pct}% dari kuantitas yang diinspeksi.`
}

function describeAchievement(rows) {
  const withPct = rows.filter((r) => r.achievement_pct != null)
  if (withPct.length === 0) return 'Belum ada work order yang bisa dihitung pencapaiannya.'
  const last = withPct[withPct.length - 1]
  const pct = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 1 }).format(last.achievement_pct)
  return `Pencapaian ${last.month.slice(0, 7)}: ${pct}% (realisasi hanya dari work order yang sudah selesai).`
}

function describeResolveHours(rows) {
  const withAvg = rows.filter((r) => r.avg_resolve_hours != null)
  if (withAvg.length === 0) return 'Belum ada tiket selesai yang bisa dihitung durasinya.'
  const last = withAvg[withAvg.length - 1]
  const hours = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 1 }).format(last.avg_resolve_hours)
  return `Rata-rata penyelesaian ${last.month.slice(0, 7)}: ${hours} jam.`
}

function describeKpiSpread(rows) {
  if (rows.length === 0) return ''
  const period = rows[0].period
  const widest = rows.reduce((worst, r) =>
    Number(r.max_score) - Number(r.min_score) > Number(worst.max_score) - Number(worst.min_score) ? r : worst,
  )
  const spread = Number(widest.max_score) - Number(widest.min_score)
  if (spread === 0) return `Periode ${period}. Nilai di tiap departemen seragam.`
  return `Periode ${period}. Sebaran terlebar: ${widest.department} (${Number(widest.min_score)}–${Number(widest.max_score)}).`
}

function describeAvgKpiScore(rows) {
  const withAvg = rows.filter((r) => r.avg_score != null)
  if (withAvg.length === 0) return 'Belum ada penilaian yang disetujui, jadi belum ada rata-rata nilai.'
  const last = withAvg[withAvg.length - 1]
  const score = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 1 }).format(last.avg_score)
  return `Rata-rata nilai ${last.period}: ${score} (hanya penilaian yang sudah disetujui).`
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

// Pengiriman selesai vs dibatalkan. Ini BUKAN dikotomi masuk-vs-keluar yang
// dipakai chart Finance/Stock, tapi pasangan biru/oranye yang sama tetap dipakai
// karena maknanya tetap "dua sisi berlawanan dari satu proses" -- dan menambah
// hue kategorikal ketiga cuma untuk chart ini akan membuat halaman ini punya
// lebih banyak warna daripada makna. Hijau/merah SENGAJA dihindari (di seluruh
// halaman ini) supaya tidak terbaca sebagai penilaian baik/buruk.
// Chart ketujuh & kedelapan (HR). Cuti dipecah per jenis karena yang menarik
// bukan totalnya, melainkan komposisinya: cuti tahunan itu hak yang memang
// dipakai, sementara sakit yang menumpuk dan tanpa gaji punya arti berbeda.
const HR_LEAVE_SERIES = [
  { key: 'annual_days', label: 'Tahunan', color: 'var(--bs-primary)' },
  { key: 'sick_days', label: 'Sakit', color: 'var(--bs-orange)' },
  { key: 'unpaid_days', label: 'Tanpa Gaji', color: 'var(--bs-danger)' },
  { key: 'other_days', label: 'Lainnya', color: 'var(--bs-secondary)' },
]

// Sebaran rating, bukan rata-rata sebagai batang: rata-rata sudah ditampilkan
// sebagai teks di bawah judul (satuannya poin, tidak bisa berbagi sumbu Y
// dengan cacah orang) -- pola yang sama dengan avg_delivery_hours di fleet.
const HR_KPI_SERIES = [
  { key: 'sangat_baik_count', label: 'Sangat Baik', color: 'var(--bs-success)' },
  { key: 'baik_count', label: 'Baik', color: 'var(--bs-primary)' },
  { key: 'cukup_count', label: 'Cukup', color: 'var(--bs-orange)' },
  { key: 'perlu_perbaikan_count', label: 'Perlu Perbaikan', color: 'var(--bs-danger)' },
]

// Chart kesembilan: satu seri saja (rata-rata nilai per departemen). Rentang
// min-max ditampilkan sebagai teks di bawah grafik, bukan seri tambahan --
// tiga batang berdampingan untuk min/rata/max mudah terbaca sebagai "tiga
// kelompok berbeda" padahal ketiganya menggambarkan kelompok yang sama.
const HR_KPI_DEPARTMENT_SERIES = [{ key: 'avg_score', label: 'Rata-rata Nilai', color: 'var(--bs-primary)' }]

// Chart 10-13, mengisi empat fact table yang datanya sudah lama masuk warehouse
// tapi belum pernah punya grafik.
const QC_SERIES = [
  { key: 'pass_count', label: 'Lolos', color: 'var(--bs-success)' },
  { key: 'partial_count', label: 'Sebagian', color: 'var(--bs-orange)' },
  { key: 'fail_count', label: 'Gagal', color: 'var(--bs-danger)' },
]

const PRODUCTION_SERIES = [
  { key: 'quantity_planned', label: 'Rencana', color: 'var(--bs-secondary)' },
  { key: 'quantity_produced', label: 'Realisasi', color: 'var(--bs-primary)' },
]

const PURCHASING_SUPPLIER_SERIES = [
  { key: 'total_spend', label: 'Belanja', color: 'var(--bs-primary)' },
]

const TICKETING_SERIES = [
  { key: 'resolved_count', label: 'Selesai', color: 'var(--bs-primary)' },
  { key: 'open_count', label: 'Belum selesai', color: 'var(--bs-orange)' },
]

// Chart 14-17: empat fact table terakhir yang belum punya grafik. Dengan ini
// seluruh 16 fact table di warehouse terwakili di dashboard.
const PAYROLL_SERIES = [
  { key: 'total_net', label: 'Gaji Bersih', color: 'var(--bs-primary)' },
  { key: 'total_deduction', label: 'Potongan', color: 'var(--bs-orange)' },
]

const ASSET_MAINTENANCE_SERIES = [
  { key: 'completed_count', label: 'Selesai', color: 'var(--bs-success)' },
  { key: 'overdue_count', label: 'Terlambat', color: 'var(--bs-danger)' },
  { key: 'cancelled_count', label: 'Dibatalkan', color: 'var(--bs-secondary)' },
]

const ECOMMERCE_SERIES = [{ key: 'revenue', label: 'Penjualan', color: 'var(--bs-primary)' }]

const FLEET_DELIVERY_SERIES = [
  { key: 'delivered_count', label: 'Selesai', color: 'var(--bs-primary)' },
  { key: 'cancelled_count', label: 'Dibatalkan', color: 'var(--bs-orange)' },
]

// Halaman ini sempat memanjang jadi 9 grafik dalam satu kolom. Dipecah per
// tema, bukan per sumber data (mis. "real-time" vs "data warehouse"): yang
// membuka dashboard mencari jawaban tentang keuangan atau SDM-nya, bukan
// tentang dari service mana angkanya diambil.
const BI_TABS = [
  { id: 'ringkasan', label: 'Ringkasan' },
  { id: 'keuangan', label: 'Keuangan & Penjualan' },
  { id: 'operasional', label: 'Operasional' },
  { id: 'sdm', label: 'SDM' },
]

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
  const [fleetDelivery, setFleetDelivery] = useState(null)
  const [fleetDeliveryError, setFleetDeliveryError] = useState('')
  const [hrLeave, setHrLeave] = useState(null)
  const [hrLeaveError, setHrLeaveError] = useState('')
  const [hrKpi, setHrKpi] = useState(null)
  const [hrKpiError, setHrKpiError] = useState('')
  const [hrKpiDept, setHrKpiDept] = useState(null)
  const [hrKpiDeptError, setHrKpiDeptError] = useState('')
  const [qc, setQc] = useState(null)
  const [qcError, setQcError] = useState('')
  const [production, setProduction] = useState(null)
  const [productionError, setProductionError] = useState('')
  const [purchasingSupplier, setPurchasingSupplier] = useState(null)
  const [purchasingSupplierError, setPurchasingSupplierError] = useState('')
  const [ticketing, setTicketing] = useState(null)
  const [ticketingError, setTicketingError] = useState('')
  const [payroll, setPayroll] = useState(null)
  const [payrollError, setPayrollError] = useState('')
  const [assetMaintenance, setAssetMaintenance] = useState(null)
  const [assetMaintenanceError, setAssetMaintenanceError] = useState('')
  const [iotDevices, setIotDevices] = useState(null)
  const [iotDevicesError, setIotDevicesError] = useState('')
  const [ecommerce, setEcommerce] = useState(null)
  const [ecommerceError, setEcommerceError] = useState('')
  const [activeTab, setActiveTab] = useState('ringkasan')
  // '' berarti "biarkan backend memilih periode terakhir yang sudah final".
  const [kpiDeptPeriod, setKpiDeptPeriod] = useState('')

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

  // Chart keenam dari dw-service. avg_delivery_hours ikut dikembalikan endpoint
  // (nullable -- bulan tanpa pengiriman selesai tidak punya rata-rata) dan
  // ditampilkan sebagai teks ringkas di bawah judul, BUKAN sebagai seri ketiga:
  // satuannya jam sementara dua seri lainnya cacah, jadi tidak bisa berbagi
  // sumbu Y yang sama.
  function loadFleetDelivery(cid) {
    setFleetDeliveryError('')
    apiClient
      .get('/api/dw/analytics/fleet-delivery-monthly-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setFleetDelivery(
          data.map((d) => ({
            ...d,
            delivered_count: Number(d.delivered_count),
            cancelled_count: Number(d.cancelled_count),
            avg_delivery_hours: d.avg_delivery_hours == null ? null : Number(d.avg_delivery_hours),
          })),
        ),
      )
      .catch(() => setFleetDeliveryError('Gagal memuat ringkasan pengiriman. Pastikan dw-service aktif.'))
  }

  // Chart ketujuh: hari cuti per bulan, dipecah per jenis. Backend hanya
  // menjumlahkan hari dari cuti APPROVED (yang ditolak tidak pernah diambil),
  // tapi total_requests ikut menghitung semua status -- itu beban administrasi
  // yang nyata, jadi ditampilkan sebagai teks, bukan batang.
  function loadHrLeave(cid) {
    setHrLeaveError('')
    apiClient
      .get('/api/dw/analytics/hr-leave-monthly-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setHrLeave(
          data.map((d) => ({
            ...d,
            annual_days: Number(d.annual_days),
            sick_days: Number(d.sick_days),
            unpaid_days: Number(d.unpaid_days),
            other_days: Number(d.other_days),
          })),
        ),
      )
      .catch(() => setHrLeaveError('Gagal memuat ringkasan cuti. Pastikan dw-service aktif.'))
  }

  // Chart kedelapan: sebaran rating KPI per periode.
  function loadHrKpi(cid) {
    setHrKpiError('')
    apiClient
      .get('/api/dw/analytics/hr-kpi-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setHrKpi(
          data.map((d) => ({
            ...d,
            sangat_baik_count: Number(d.sangat_baik_count),
            baik_count: Number(d.baik_count),
            cukup_count: Number(d.cukup_count),
            perlu_perbaikan_count: Number(d.perlu_perbaikan_count),
            avg_score: d.avg_score == null ? null : Number(d.avg_score),
          })),
        ),
      )
      .catch(() => setHrKpiError('Gagal memuat ringkasan KPI. Pastikan dw-service aktif.'))
  }

  // Chart kesembilan: perbandingan antar departemen pada SATU periode. Tanpa
  // parameter period, backend memilih periode terakhir yang punya penilaian
  // disetujui -- UI tidak perlu menebak periode mana yang sudah final.
  function loadHrKpiDept(cid, period = '') {
    setHrKpiDeptError('')
    apiClient
      .get('/api/dw/analytics/hr-kpi-department-summary', {
        params: period ? { company_id: cid, period } : { company_id: cid },
      })
      .then(({ data }) =>
        setHrKpiDept(
          data.map((d) => ({
            ...d,
            avg_score: d.avg_score == null ? 0 : Number(d.avg_score),
            min_score: Number(d.min_score),
            max_score: Number(d.max_score),
          })),
        ),
      )
      .catch(() => setHrKpiDeptError('Gagal memuat KPI per departemen. Pastikan dw-service aktif.'))
  }

  function loadQc(cid) {
    setQcError('')
    apiClient
      .get('/api/dw/analytics/qc-monthly-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setQc(
          data.map((d) => ({
            ...d,
            pass_count: Number(d.pass_count),
            partial_count: Number(d.partial_count),
            fail_count: Number(d.fail_count),
            defect_rate_pct: d.defect_rate_pct == null ? null : Number(d.defect_rate_pct),
          })),
        ),
      )
      .catch(() => setQcError('Gagal memuat ringkasan QC. Pastikan dw-service aktif.'))
  }

  function loadProduction(cid) {
    setProductionError('')
    apiClient
      .get('/api/dw/analytics/production-monthly-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setProduction(
          data.map((d) => ({
            ...d,
            quantity_planned: Number(d.quantity_planned),
            quantity_produced: Number(d.quantity_produced),
            achievement_pct: d.achievement_pct == null ? null : Number(d.achievement_pct),
          })),
        ),
      )
      .catch(() => setProductionError('Gagal memuat ringkasan produksi. Pastikan dw-service aktif.'))
  }

  function loadPurchasingSupplier(cid) {
    setPurchasingSupplierError('')
    apiClient
      .get('/api/dw/analytics/purchasing-supplier-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setPurchasingSupplier(data.map((d) => ({ ...d, total_spend: Number(d.total_spend) }))),
      )
      .catch(() => setPurchasingSupplierError('Gagal memuat belanja per supplier. Pastikan dw-service aktif.'))
  }

  function loadTicketing(cid) {
    setTicketingError('')
    apiClient
      .get('/api/dw/analytics/ticketing-monthly-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setTicketing(
          data.map((d) => ({
            ...d,
            resolved_count: Number(d.resolved_count),
            open_count: Number(d.open_count),
            avg_resolve_hours: d.avg_resolve_hours == null ? null : Number(d.avg_resolve_hours),
          })),
        ),
      )
      .catch(() => setTicketingError('Gagal memuat ringkasan tiket. Pastikan dw-service aktif.'))
  }

  function loadPayroll(cid) {
    setPayrollError('')
    apiClient
      .get('/api/dw/analytics/payroll-period-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setPayroll(
          data.map((d) => ({
            ...d,
            total_net: Number(d.total_net),
            total_deduction: Number(d.total_deduction),
            attendance_pct: d.attendance_pct == null ? null : Number(d.attendance_pct),
          })),
        ),
      )
      .catch(() => setPayrollError('Gagal memuat ringkasan payroll. Pastikan dw-service aktif.'))
  }

  function loadAssetMaintenance(cid) {
    setAssetMaintenanceError('')
    apiClient
      .get('/api/dw/analytics/asset-maintenance-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setAssetMaintenance(
          data.map((d) => ({
            ...d,
            completed_count: Number(d.completed_count),
            overdue_count: Number(d.overdue_count),
            cancelled_count: Number(d.cancelled_count),
            avg_delay_days: d.avg_delay_days == null ? null : Number(d.avg_delay_days),
          })),
        ),
      )
      .catch(() => setAssetMaintenanceError('Gagal memuat ringkasan perawatan aset. Pastikan dw-service aktif.'))
  }

  // Sensor ditampilkan sebagai tabel, bukan grafik: satuannya berbeda-beda per
  // jenis pembacaan (derajat, persen, ppm), jadi satu sumbu Y bersama akan
  // menyesatkan.
  function loadIotDevices(cid) {
    setIotDevicesError('')
    apiClient
      .get('/api/dw/analytics/iot-device-summary', { params: { company_id: cid } })
      .then(({ data }) => setIotDevices(data))
      .catch(() => setIotDevicesError('Gagal memuat ringkasan sensor. Pastikan dw-service aktif.'))
  }

  function loadEcommerce(cid) {
    setEcommerceError('')
    apiClient
      .get('/api/dw/analytics/ecommerce-monthly-summary', { params: { company_id: cid } })
      .then(({ data }) =>
        setEcommerce(
          data.map((d) => ({
            ...d,
            revenue: Number(d.revenue),
            avg_order_value: d.avg_order_value == null ? null : Number(d.avg_order_value),
          })),
        ),
      )
      .catch(() => setEcommerceError('Gagal memuat ringkasan penjualan online. Pastikan dw-service aktif.'))
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
    loadFleetDelivery(companyId)
    loadHrLeave(companyId)
    loadHrKpi(companyId)
    loadHrKpiDept(companyId)
    loadQc(companyId)
    loadProduction(companyId)
    loadPurchasingSupplier(companyId)
    loadTicketing(companyId)
    loadPayroll(companyId)
    loadAssetMaintenance(companyId)
    loadIotDevices(companyId)
    loadEcommerce(companyId)
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

          <ul className="nav edp-filter-tabs">
            {BI_TABS.map((tab) => (
              <li className="nav-item" key={tab.id}>
                <button
                  type="button"
                  className={`edp-filter-tab ${activeTab === tab.id ? 'active' : ''}`}
                  onClick={() => setActiveTab(tab.id)}
                >
                  {tab.label}
                </button>
              </li>
            ))}
          </ul>

          {activeTab === 'ringkasan' && (
            <>
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


            </>
          )}

          {activeTab === 'keuangan' && (
            <>
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


            </>
          )}

          {activeTab === 'keuangan' && (
            <div className="row g-3">
              <div className="col-md-8">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Belanja per Supplier (dari Data Warehouse)</h6>
                  <div className="text-secondary small mb-3">
                    Hanya PO yang sudah diterima atau ditagih &mdash; yang masih draft/dikonfirmasi baru rencana.
                  </div>
                  {purchasingSupplierError && (
                    <div className="alert alert-warning py-2 small mb-0">{purchasingSupplierError}</div>
                  )}
                  {!purchasingSupplierError && purchasingSupplier == null && (
                    <div className="text-secondary small">Memuat...</div>
                  )}
                  {!purchasingSupplierError && purchasingSupplier != null && purchasingSupplier.length === 0 && (
                    <div className="text-secondary small">Belum ada PO yang diterima atau ditagih.</div>
                  )}
                  {!purchasingSupplierError && purchasingSupplier != null && purchasingSupplier.length > 0 && (
                    <GroupedBarChart
                      data={purchasingSupplier}
                      series={PURCHASING_SUPPLIER_SERIES}
                      formatValue={(v) => `Rp ${formatMoney(v)}`}
                      categoryKey="supplier_code"
                      formatCategoryTick={(v) => v}
                    />
                  )}
                </div>
              </div>

              <div className="col-md-4">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Penjualan Online per Bulan (dari Data Warehouse)</h6>
                  {!ecommerceError && ecommerce != null && ecommerce.length > 0 && (
                    <div className="text-secondary small mb-3">{describeAvgOrderValue(ecommerce)}</div>
                  )}
                  {ecommerceError && <div className="alert alert-warning py-2 small mb-0">{ecommerceError}</div>}
                  {!ecommerceError && ecommerce == null && <div className="text-secondary small">Memuat...</div>}
                  {!ecommerceError && ecommerce != null && ecommerce.length === 0 && (
                    <div className="text-secondary small">Belum ada order online yang tercatat.</div>
                  )}
                  {!ecommerceError && ecommerce != null && ecommerce.length > 0 && (
                    <GroupedBarChart
                      data={ecommerce}
                      series={ECOMMERCE_SERIES}
                      formatValue={(v) => `Rp ${formatMoney(v)}`}
                    />
                  )}
                </div>
              </div>
            </div>
          )}

          {activeTab === 'operasional' && (
            <>
            <div className="row g-3">
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
            </div>

            <div className="row g-3">
              <div className="col-md-8">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Pengiriman per Bulan (dari Data Warehouse)</h6>
                  {!fleetDeliveryError && fleetDelivery != null && fleetDelivery.length > 0 && (
                    <div className="text-secondary small mb-3">{describeAvgDeliveryHours(fleetDelivery)}</div>
                  )}
                  {fleetDeliveryError && <div className="alert alert-warning py-2 small mb-0">{fleetDeliveryError}</div>}
                  {!fleetDeliveryError && fleetDelivery == null && <div className="text-secondary small">Memuat...</div>}
                  {!fleetDeliveryError && fleetDelivery != null && fleetDelivery.length === 0 && (
                    <div className="text-secondary small">Belum ada surat jalan yang tercatat.</div>
                  )}
                  {!fleetDeliveryError && fleetDelivery != null && fleetDelivery.length > 0 && (
                    <GroupedBarChart
                      data={fleetDelivery}
                      series={FLEET_DELIVERY_SERIES}
                      formatValue={(v) => formatQty(v)}
                    />
                  )}
                </div>
              </div>
            </div>


            </>
          )}

          {activeTab === 'operasional' && (
            <div className="row g-3">
              <div className="col-md-6">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Hasil Inspeksi QC per Bulan (dari Data Warehouse)</h6>
                  {!qcError && qc != null && qc.length > 0 && (
                    <div className="text-secondary small mb-3">{describeDefectRate(qc)}</div>
                  )}
                  {qcError && <div className="alert alert-warning py-2 small mb-0">{qcError}</div>}
                  {!qcError && qc == null && <div className="text-secondary small">Memuat...</div>}
                  {!qcError && qc != null && qc.length === 0 && (
                    <div className="text-secondary small">Belum ada inspeksi yang tercatat.</div>
                  )}
                  {!qcError && qc != null && qc.length > 0 && (
                    <GroupedBarChart data={qc} series={QC_SERIES} formatValue={(v) => formatQty(v)} />
                  )}
                </div>
              </div>

              <div className="col-md-6">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Rencana vs Realisasi Produksi (dari Data Warehouse)</h6>
                  {!productionError && production != null && production.length > 0 && (
                    <div className="text-secondary small mb-3">{describeAchievement(production)}</div>
                  )}
                  {productionError && <div className="alert alert-warning py-2 small mb-0">{productionError}</div>}
                  {!productionError && production == null && <div className="text-secondary small">Memuat...</div>}
                  {!productionError && production != null && production.length === 0 && (
                    <div className="text-secondary small">Belum ada work order yang tercatat.</div>
                  )}
                  {!productionError && production != null && production.length > 0 && (
                    <GroupedBarChart data={production} series={PRODUCTION_SERIES} formatValue={(v) => formatQty(v)} />
                  )}
                </div>
              </div>

              <div className="col-md-6">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Tiket per Bulan (dari Data Warehouse)</h6>
                  {!ticketingError && ticketing != null && ticketing.length > 0 && (
                    <div className="text-secondary small mb-3">{describeResolveHours(ticketing)}</div>
                  )}
                  {ticketingError && <div className="alert alert-warning py-2 small mb-0">{ticketingError}</div>}
                  {!ticketingError && ticketing == null && <div className="text-secondary small">Memuat...</div>}
                  {!ticketingError && ticketing != null && ticketing.length === 0 && (
                    <div className="text-secondary small">Belum ada tiket yang tercatat.</div>
                  )}
                  {!ticketingError && ticketing != null && ticketing.length > 0 && (
                    <GroupedBarChart data={ticketing} series={TICKETING_SERIES} formatValue={(v) => formatQty(v)} />
                  )}
                </div>
              </div>
            </div>
          )}

          {activeTab === 'operasional' && (
            <div className="row g-3">
              <div className="col-md-6">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Perawatan Aset per Bulan (dari Data Warehouse)</h6>
                  {!assetMaintenanceError && assetMaintenance != null && assetMaintenance.length > 0 && (
                    <div className="text-secondary small mb-3">{describeMaintenanceDelay(assetMaintenance)}</div>
                  )}
                  {assetMaintenanceError && (
                    <div className="alert alert-warning py-2 small mb-0">{assetMaintenanceError}</div>
                  )}
                  {!assetMaintenanceError && assetMaintenance == null && (
                    <div className="text-secondary small">Memuat...</div>
                  )}
                  {!assetMaintenanceError && assetMaintenance != null && assetMaintenance.length === 0 && (
                    <div className="text-secondary small">Belum ada jadwal perawatan yang tercatat.</div>
                  )}
                  {!assetMaintenanceError && assetMaintenance != null && assetMaintenance.length > 0 && (
                    <GroupedBarChart
                      data={assetMaintenance}
                      series={ASSET_MAINTENANCE_SERIES}
                      formatValue={(v) => formatQty(v)}
                    />
                  )}
                </div>
              </div>

              <div className="col-md-6">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Sensor IoT (dari Data Warehouse)</h6>
                  <div className="text-secondary small mb-3">
                    Dikelompokkan per device &amp; jenis pembacaan &mdash; satuannya berbeda-beda, jadi
                    ditampilkan sebagai tabel, bukan grafik dengan satu sumbu bersama.
                  </div>
                  {iotDevicesError && <div className="alert alert-warning py-2 small mb-0">{iotDevicesError}</div>}
                  {!iotDevicesError && iotDevices == null && <div className="text-secondary small">Memuat...</div>}
                  {!iotDevicesError && iotDevices != null && iotDevices.length === 0 && (
                    <div className="text-secondary small">Belum ada pembacaan sensor yang tercatat.</div>
                  )}
                  {!iotDevicesError && iotDevices != null && iotDevices.length > 0 && (
                    <div className="table-responsive">
                      <table className="table table-sm align-middle mb-0">
                        <thead>
                          <tr>
                            <th>Device</th>
                            <th>Pembacaan</th>
                            <th className="text-end">Jumlah</th>
                            <th className="text-end">Rata-rata</th>
                            <th className="text-end">Min-Max</th>
                            <th>Terakhir</th>
                          </tr>
                        </thead>
                        <tbody>
                          {iotDevices.map((d) => (
                            <tr key={`${d.device_code}-${d.reading_type}`}>
                              <td>
                                <code>{d.device_code}</code>
                              </td>
                              <td className="text-secondary">{d.reading_type}</td>
                              <td className="text-end">{formatQty(d.reading_count)}</td>
                              <td className="text-end">
                                {d.avg_value == null
                                  ? '—'
                                  : new Intl.NumberFormat('id-ID', { maximumFractionDigits: 2 }).format(d.avg_value)}
                              </td>
                              <td className="text-end text-secondary small">
                                {d.min_value == null ? '—' : `${Number(d.min_value)} – ${Number(d.max_value)}`}
                              </td>
                              <td className="text-secondary small">{d.last_read_at?.slice(0, 16) ?? '—'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}

          {activeTab === 'sdm' && (
            <>
            <div className="row g-3">
              <div className="col-md-6">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Hari Cuti per Bulan (dari Data Warehouse)</h6>
                  {!hrLeaveError && hrLeave != null && hrLeave.length > 0 && (
                    <div className="text-secondary small mb-3">
                      Hanya cuti yang disetujui yang dihitung harinya; pengajuan yang ditolak &amp; dibatalkan tetap
                      masuk hitungan jumlah pengajuan.
                    </div>
                  )}
                  {hrLeaveError && <div className="alert alert-warning py-2 small mb-0">{hrLeaveError}</div>}
                  {!hrLeaveError && hrLeave == null && <div className="text-secondary small">Memuat...</div>}
                  {!hrLeaveError && hrLeave != null && hrLeave.length === 0 && (
                    <div className="text-secondary small">Belum ada pengajuan cuti yang tercatat.</div>
                  )}
                  {!hrLeaveError && hrLeave != null && hrLeave.length > 0 && (
                    <GroupedBarChart data={hrLeave} series={HR_LEAVE_SERIES} formatValue={(v) => formatQty(v)} />
                  )}
                </div>
              </div>

              <div className="col-md-6">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Sebaran Rating KPI (dari Data Warehouse)</h6>
                  {!hrKpiError && hrKpi != null && hrKpi.length > 0 && (
                    <div className="text-secondary small mb-3">{describeAvgKpiScore(hrKpi)}</div>
                  )}
                  {hrKpiError && <div className="alert alert-warning py-2 small mb-0">{hrKpiError}</div>}
                  {!hrKpiError && hrKpi == null && <div className="text-secondary small">Memuat...</div>}
                  {!hrKpiError && hrKpi != null && hrKpi.length === 0 && (
                    <div className="text-secondary small">Belum ada penilaian KPI yang tercatat.</div>
                  )}
                  {!hrKpiError && hrKpi != null && hrKpi.length > 0 && (
                    <GroupedBarChart
                      data={hrKpi}
                      series={HR_KPI_SERIES}
                      formatValue={(v) => formatQty(v)}
                      categoryKey="period"
                      formatCategoryTick={(v) => v}
                    />
                  )}
                </div>
              </div>
            </div>


            <div className="row g-3">
              <div className="col-md-6">
                <div className="card p-3 h-100">
                  <div className="d-flex align-items-center justify-content-between gap-2 mb-1">
                  <h6 className="mb-0">Rata-rata KPI per Departemen (dari Data Warehouse)</h6>
                  <select
                    className="form-select form-select-sm w-auto"
                    aria-label="Periode KPI"
                    value={kpiDeptPeriod}
                    onChange={(e) => {
                      setKpiDeptPeriod(e.target.value)
                      setHrKpiDept(null)
                      loadHrKpiDept(companyId, e.target.value)
                    }}
                  >
                    <option value="">Periode terakhir</option>
                    {(hrKpi ?? [])
                      .filter((p) => p.approved_count > 0)
                      .map((p) => (
                        <option key={p.period} value={p.period}>
                          {p.period}
                        </option>
                      ))}
                  </select>
                </div>
                  {!hrKpiDeptError && hrKpiDept != null && hrKpiDept.length > 0 && (
                    <div className="text-secondary small mb-3">{describeKpiSpread(hrKpiDept)}</div>
                  )}
                  {hrKpiDeptError && <div className="alert alert-warning py-2 small mb-0">{hrKpiDeptError}</div>}
                  {!hrKpiDeptError && hrKpiDept == null && <div className="text-secondary small">Memuat...</div>}
                  {!hrKpiDeptError && hrKpiDept != null && hrKpiDept.length === 0 && (
                    <div className="text-secondary small">
                      Belum ada penilaian KPI yang disetujui. Perbandingan antar departemen baru muncul setelah
                      ada penilaian yang final di satu periode.
                    </div>
                  )}
                  {!hrKpiDeptError && hrKpiDept != null && hrKpiDept.length > 0 && (
                    <GroupedBarChart
                      data={hrKpiDept}
                      series={HR_KPI_DEPARTMENT_SERIES}
                      formatValue={(v) => new Intl.NumberFormat('id-ID', { maximumFractionDigits: 1 }).format(v)}
                      categoryKey="department"
                      formatCategoryTick={(v) => v}
                    />
                  )}
                </div>
              </div>
            </div>


            </>
          )}

          {activeTab === 'sdm' && (
            <div className="row g-3">
              <div className="col-md-8">
                <div className="card p-3 h-100">
                  <h6 className="mb-1">Payroll per Periode (dari Data Warehouse)</h6>
                  {!payrollError && payroll != null && payroll.length > 0 && (
                    <div className="text-secondary small mb-3">{describeAttendance(payroll)}</div>
                  )}
                  {payrollError && <div className="alert alert-warning py-2 small mb-0">{payrollError}</div>}
                  {!payrollError && payroll == null && <div className="text-secondary small">Memuat...</div>}
                  {!payrollError && payroll != null && payroll.length === 0 && (
                    <div className="text-secondary small">
                      Belum ada payroll yang diposting ke GL. Hanya run POSTED yang ikut dihitung.
                    </div>
                  )}
                  {!payrollError && payroll != null && payroll.length > 0 && (
                    <GroupedBarChart
                      data={payroll}
                      series={PAYROLL_SERIES}
                      formatValue={(v) => `Rp ${formatMoney(v)}`}
                      categoryKey="period"
                      formatCategoryTick={(v) => v}
                    />
                  )}
                </div>
              </div>
            </div>
          )}

          <div className="text-secondary small">Terakhir dimuat: {new Date(summary.generated_at).toLocaleString('id-ID')}</div>
        </>
      )}
    </div>
  )
}

export default BIDashboardsPage
