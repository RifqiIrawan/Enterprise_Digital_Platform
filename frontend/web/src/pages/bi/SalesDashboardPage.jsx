import RoleDashboard, { formatMoney, formatCount } from './RoleDashboard.jsx'
import { SALES_SERIES, CRM_PIPELINE_SERIES, ECOMMERCE_SERIES, FLEET_DELIVERY_SERIES } from './chartSeries.js'

// Finance ikut diminta karena angka piutang adalah bagian dari pekerjaan Sales
// (order yang sudah dikirim tapi belum dibayar), bukan urusan Finance semata.
const SECTIONS = ['sales', 'finance']

const CHARTS = [
  {
    endpoint: 'sales-monthly-summary',
    title: 'Nilai Penjualan (bulanan)',
    series: SALES_SERIES,
    format: formatMoney,
  },
  {
    endpoint: 'crm-pipeline-summary',
    title: 'Pipeline CRM per Stage',
    note: 'Nilai terbobot = nilai deal dikalikan probability-nya; urutan stage mengikuti funnel, bukan alfabetis.',
    series: CRM_PIPELINE_SERIES,
    categoryKey: 'stage',
    tick: (v) => v,
    format: formatMoney,
  },
  {
    endpoint: 'ecommerce-monthly-summary',
    title: 'Penjualan E-commerce (bulanan)',
    series: ECOMMERCE_SERIES,
    format: formatMoney,
  },
  {
    endpoint: 'fleet-delivery-monthly-summary',
    title: 'Pengiriman: Selesai vs Dibatalkan (bulanan)',
    series: FLEET_DELIVERY_SERIES,
  },
]

function tiles(summary) {
  return [
    {
      icon: 'bi-cash-coin',
      label: 'Nilai Penjualan',
      value: formatMoney(summary.sales?.total_revenue),
      hint: `${formatCount(summary.sales?.total_orders)} sales order`,
    },
    {
      icon: 'bi-hourglass-split',
      label: 'Piutang Outstanding',
      value: formatMoney(summary.finance?.ar_outstanding),
      hint: `dari ${formatMoney(summary.finance?.ar_total)} total piutang`,
      color: 'amber',
    },
    {
      icon: 'bi-check2-circle',
      label: 'Order Terkirim',
      value: formatCount(summary.sales?.by_status?.FULFILLED),
      hint: `${formatCount(summary.sales?.by_status?.INVOICED)} sudah diinvoice`,
      color: 'green',
    },
    {
      icon: 'bi-pencil-square',
      label: 'Order Draft',
      value: formatCount(summary.sales?.by_status?.DRAFT),
      hint: `${formatCount(summary.sales?.by_status?.CONFIRMED)} dikonfirmasi`,
      color: 'blue',
    },
  ]
}

function SalesDashboardPage() {
  return (
    <RoleDashboard
      title="Dashboard Sales"
      description="Penjualan, pipeline CRM, e-commerce, dan pengiriman — beserta piutang yang belum tertagih."
      sections={SECTIONS}
      tiles={tiles}
      charts={CHARTS}
    />
  )
}

export default SalesDashboardPage
