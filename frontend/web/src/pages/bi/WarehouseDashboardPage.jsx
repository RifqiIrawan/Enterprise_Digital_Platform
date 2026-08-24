import RoleDashboard, { formatMoney, formatCount } from './RoleDashboard.jsx'
import { STOCK_SERIES, PURCHASING_SUPPLIER_SERIES } from './chartSeries.js'

// Gudang dan pembelian dibaca bersama: arus barang masuk berasal dari PO yang
// diterima, jadi memisahkan keduanya ke dua dashboard hanya memaksa orang yang
// sama membuka dua layar.
const SECTIONS = ['warehouse', 'purchasing']

const CHARTS = [
  {
    endpoint: 'stock-movement-monthly-summary',
    title: 'Stock In vs Stock Out (bulanan)',
    series: STOCK_SERIES,
  },
  {
    endpoint: 'purchasing-supplier-summary',
    title: 'Belanja per Pemasok',
    note: 'Diurutkan dari belanja terbesar oleh dw-service.',
    series: PURCHASING_SUPPLIER_SERIES,
    categoryKey: 'supplier_code',
    tick: (v) => v,
    format: formatMoney,
  },
]

function tiles(summary) {
  return [
    {
      icon: 'bi-box-seam',
      label: 'Produk',
      value: formatCount(summary.warehouse?.total_products),
      hint: `${formatCount(summary.warehouse?.total_warehouses)} gudang`,
    },
    {
      icon: 'bi-exclamation-triangle',
      label: 'Stok Menipis',
      value: formatCount(summary.warehouse?.low_stock_count),
      hint: 'baris stok di bawah ambang sementara (10)',
      color: 'amber',
    },
    {
      icon: 'bi-list-ol',
      label: 'Baris Stok',
      value: formatCount(summary.warehouse?.total_stock_lines),
      hint: 'kombinasi produk × gudang',
      color: 'blue',
    },
    {
      icon: 'bi-truck',
      label: 'Belanja Pembelian',
      value: formatMoney(summary.purchasing?.total_spend),
      hint: `${formatCount(summary.purchasing?.total_orders)} purchase order`,
      color: 'violet',
    },
  ]
}

function WarehouseDashboardPage() {
  return (
    <RoleDashboard
      title="Dashboard Gudang"
      description="Arus barang masuk dan keluar, kondisi stok, dan belanja pembelian per pemasok."
      sections={SECTIONS}
      tiles={tiles}
      charts={CHARTS}
    />
  )
}

export default WarehouseDashboardPage
