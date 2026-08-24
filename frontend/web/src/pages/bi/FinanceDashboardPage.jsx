import RoleDashboard, { formatMoney, formatCount } from './RoleDashboard.jsx'
import { FINANCE_SERIES, PAYROLL_SERIES, PROJECT_COST_SERIES } from './chartSeries.js'

// Sales & purchasing ikut diminta karena piutang dan hutang berasal dari sana;
// tanpa keduanya, kartu AR/AP tidak punya konteks berapa order yang
// menghasilkannya.
const SECTIONS = ['finance', 'sales', 'purchasing']

const CHARTS = [
  {
    endpoint: 'finance-monthly-summary',
    title: 'Revenue vs Expense (bulanan)',
    note: 'Dari jurnal yang sudah POSTED saja.',
    series: FINANCE_SERIES,
    format: formatMoney,
  },
  {
    endpoint: 'payroll-period-summary',
    title: 'Beban Gaji per Periode',
    note: 'Gaji bersih dan potongan dari payroll run yang sudah diposting.',
    series: PAYROLL_SERIES,
    categoryKey: 'period',
    tick: (v) => String(v).slice(2, 7),
    format: formatMoney,
  },
  {
    endpoint: 'project-cost-summary',
    title: 'Biaya Proyek (timesheet yang sudah diposting)',
    series: PROJECT_COST_SERIES,
    categoryKey: 'project_code',
    tick: (v) => v,
    format: formatMoney,
  },
]

function tiles(summary) {
  return [
    {
      icon: 'bi-cash-coin',
      label: 'Piutang Outstanding',
      value: formatMoney(summary.finance?.ar_outstanding),
      hint: `dari ${formatMoney(summary.finance?.ar_total)}`,
    },
    {
      icon: 'bi-wallet2',
      label: 'Hutang Outstanding',
      value: formatMoney(summary.finance?.ap_outstanding),
      hint: `dari ${formatMoney(summary.finance?.ap_total)}`,
      color: 'amber',
    },
    {
      icon: 'bi-journal-text',
      label: 'Jurnal Tercatat',
      value: formatCount(summary.finance?.journal_entries_count),
      hint: 'seluruh entry, termasuk draft',
      color: 'blue',
    },
    {
      icon: 'bi-arrow-left-right',
      label: 'Order Masuk / Keluar',
      value: `${formatCount(summary.sales?.total_orders)} / ${formatCount(summary.purchasing?.total_orders)}`,
      hint: `Belanja ${formatMoney(summary.purchasing?.total_spend)}`,
      color: 'violet',
    },
  ]
}

function FinanceDashboardPage() {
  return (
    <RoleDashboard
      title="Dashboard Finance"
      description="Posisi piutang & hutang, arus revenue/expense, beban gaji, dan biaya proyek yang sudah masuk buku besar."
      sections={SECTIONS}
      tiles={tiles}
      charts={CHARTS}
    />
  )
}

export default FinanceDashboardPage
