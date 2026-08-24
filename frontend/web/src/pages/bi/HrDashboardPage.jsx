import RoleDashboard, { formatMoney, formatCount } from './RoleDashboard.jsx'
import { HR_LEAVE_SERIES, HR_KPI_SERIES, HR_KPI_DEPARTMENT_SERIES, PAYROLL_SERIES } from './chartSeries.js'

const SECTIONS = ['hr']

const periodTick = (v) => String(v).slice(2, 7)

const CHARTS = [
  {
    endpoint: 'hr-leave-monthly-summary',
    title: 'Hari Cuti per Jenis (bulanan)',
    note: 'Yang menarik bukan totalnya, melainkan komposisinya — sakit yang menumpuk dan cuti tanpa gaji punya arti berbeda dari cuti tahunan.',
    series: HR_LEAVE_SERIES,
  },
  {
    endpoint: 'hr-kpi-summary',
    title: 'Sebaran Nilai KPI per Periode',
    note: 'Hanya penilaian yang sudah disetujui.',
    series: HR_KPI_SERIES,
    categoryKey: 'period',
    tick: periodTick,
  },
  {
    endpoint: 'hr-kpi-department-summary',
    title: 'Rata-rata Nilai KPI per Departemen',
    series: HR_KPI_DEPARTMENT_SERIES,
    categoryKey: 'department',
    tick: (v) => v,
  },
  {
    endpoint: 'payroll-period-summary',
    title: 'Gaji Bersih vs Potongan per Periode',
    series: PAYROLL_SERIES,
    categoryKey: 'period',
    tick: periodTick,
    format: formatMoney,
  },
]

function tiles(summary) {
  const total = summary.hr?.total_employees ?? 0
  const active = summary.hr?.active_employees ?? 0
  return [
    {
      icon: 'bi-people',
      label: 'Karyawan Aktif',
      value: formatCount(active),
      hint: `dari ${formatCount(total)} total`,
    },
    {
      icon: 'bi-person-dash',
      label: 'Nonaktif',
      value: formatCount(total - active),
      hint: 'resign, pensiun, atau kontrak berakhir',
      color: 'amber',
    },
  ]
}

function HrDashboardPage() {
  return (
    <RoleDashboard
      title="Dashboard SDM"
      description="Komposisi cuti, sebaran nilai KPI, dan beban gaji per periode."
      sections={SECTIONS}
      tiles={tiles}
      charts={CHARTS}
    />
  )
}

export default HrDashboardPage
