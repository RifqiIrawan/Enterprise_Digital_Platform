import RoleDashboard, { formatMoney, formatCount } from './RoleDashboard.jsx'
import { FINANCE_SERIES, SALES_SERIES, PRODUCTION_SERIES, HR_KPI_SERIES } from './chartSeries.js'

// Dashboard Eksekutif adalah satu-satunya yang meminta SELURUH bagian
// ringkasan: pertanyaan direksi memang lintas fungsi. Lima dashboard lainnya
// sengaja menyempit, dan itulah sebabnya parameter ?sections= ada.
const SECTIONS = ['sales', 'purchasing', 'finance', 'warehouse', 'production', 'qc', 'hr', 'asset']

const CHARTS = [
  {
    endpoint: 'finance-monthly-summary',
    title: 'Revenue vs Expense (bulanan)',
    note: 'Dari jurnal yang sudah POSTED — angka yang benar-benar masuk buku besar, bukan draft.',
    series: FINANCE_SERIES,
    format: formatMoney,
  },
  {
    endpoint: 'sales-monthly-summary',
    title: 'Nilai Penjualan (bulanan)',
    series: SALES_SERIES,
    format: formatMoney,
  },
  {
    endpoint: 'production-monthly-summary',
    title: 'Rencana vs Realisasi Produksi (bulanan)',
    series: PRODUCTION_SERIES,
  },
  {
    endpoint: 'hr-kpi-summary',
    title: 'Sebaran Nilai KPI per Periode',
    note: 'Hanya penilaian yang sudah disetujui.',
    series: HR_KPI_SERIES,
    categoryKey: 'period',
    tick: (v) => String(v).slice(2, 7),
  },
]

function tiles(summary) {
  const qcRate = summary.qc?.pass_rate_pct ?? 0
  return [
    {
      icon: 'bi-cash-coin',
      label: 'Nilai Penjualan',
      value: formatMoney(summary.sales?.total_revenue),
      hint: `${formatCount(summary.sales?.total_orders)} sales order`,
    },
    {
      icon: 'bi-wallet2',
      label: 'Piutang Outstanding',
      value: formatMoney(summary.finance?.ar_outstanding),
      hint: `Hutang ${formatMoney(summary.finance?.ap_outstanding)}`,
      color: 'amber',
    },
    {
      icon: 'bi-gear-wide-connected',
      label: 'Work Order',
      value: formatCount(summary.production?.total_work_orders),
      hint: `Lolos QC ${qcRate.toFixed(1)}%`,
      color: 'blue',
    },
    {
      icon: 'bi-people',
      label: 'Karyawan Aktif',
      value: formatCount(summary.hr?.active_employees),
      hint: `dari ${formatCount(summary.hr?.total_employees)} total`,
      color: 'violet',
    },
  ]
}

function ExecutiveDashboardPage() {
  return (
    <RoleDashboard
      title="Dashboard Eksekutif"
      description="Ringkasan lintas modul: penjualan, posisi piutang/hutang, produksi, dan SDM dalam satu layar."
      sections={SECTIONS}
      tiles={tiles}
      charts={CHARTS}
    />
  )
}

export default ExecutiveDashboardPage
