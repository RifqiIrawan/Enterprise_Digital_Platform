import RoleDashboard, { formatCount } from './RoleDashboard.jsx'
import { PRODUCTION_SERIES, QC_SERIES, ASSET_MAINTENANCE_SERIES } from './chartSeries.js'

// Produksi, mutu, dan kondisi mesin dibaca sebagai satu gambar: work order yang
// meleset, cacat yang naik, dan maintenance yang terlambat biasanya adalah tiga
// cara melihat masalah yang sama.
const SECTIONS = ['production', 'qc', 'asset']

const CHARTS = [
  {
    endpoint: 'production-monthly-summary',
    title: 'Rencana vs Realisasi Produksi (bulanan)',
    series: PRODUCTION_SERIES,
  },
  {
    endpoint: 'qc-monthly-summary',
    title: 'Hasil Inspeksi Mutu (bulanan)',
    series: QC_SERIES,
  },
  {
    endpoint: 'asset-maintenance-summary',
    title: 'Maintenance Aset (bulanan)',
    note: 'Terlambat = jadwal sudah lewat tapi belum dikerjakan.',
    series: ASSET_MAINTENANCE_SERIES,
  },
]

function tiles(summary) {
  const qcRate = summary.qc?.pass_rate_pct ?? 0
  return [
    {
      icon: 'bi-gear-wide-connected',
      label: 'Work Order',
      value: formatCount(summary.production?.total_work_orders),
      hint: `${formatCount(summary.production?.by_status?.IN_PROGRESS)} sedang berjalan`,
    },
    {
      icon: 'bi-patch-check',
      label: 'Lolos QC',
      value: `${qcRate.toFixed(1)}%`,
      hint: `${formatCount(summary.qc?.total_inspections)} inspeksi`,
      color: 'green',
    },
    {
      icon: 'bi-x-octagon',
      label: 'Inspeksi Gagal',
      value: formatCount(summary.qc?.fail_count),
      hint: `${formatCount(summary.qc?.partial_count)} lolos sebagian`,
      color: 'rose',
    },
    {
      icon: 'bi-tools',
      label: 'Maintenance Terlambat',
      value: formatCount(summary.asset?.overdue_maintenance_count),
      hint: `${formatCount(summary.asset?.active_assets)} aset aktif`,
      color: 'amber',
    },
  ]
}

function ManufacturingDashboardPage() {
  return (
    <RoleDashboard
      title="Dashboard Manufaktur"
      description="Capaian produksi, hasil inspeksi mutu, dan kondisi perawatan mesin dalam satu layar."
      sections={SECTIONS}
      tiles={tiles}
      charts={CHARTS}
    />
  )
}

export default ManufacturingDashboardPage
