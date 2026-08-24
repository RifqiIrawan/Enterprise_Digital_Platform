import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import StatTile from '../../components/dashboard/StatTile.jsx'
import GroupedBarChart from '../aibi/GroupedBarChart.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

// Kerangka bersama keenam dashboard per peran (Fase 9). Tiap halaman peran
// hanya berisi KONFIGURASI -- bagian ringkasan mana yang dibutuhkan, kartu
// angka apa yang ditampilkan, grafik mana yang digambar -- sementara pemuatan
// data, toleransi kegagalan, dan tata letaknya tinggal satu tempat. Enam
// salinan halaman yang mirip-tapi-tidak-persis adalah cara paling mudah
// membuat dashboard Sales dan dashboard Gudang berperilaku berbeda tanpa ada
// yang memutuskan begitu.
//
// Dua sumber datanya sengaja dipisah penanganannya, mengikuti halaman BI
// Dashboards: ringkasan lintas modul dari ai-bi-service (yang sudah toleran
// terhadap service mati lewat field `errors`), dan deret waktu dari dw-service
// (ClickHouse) yang gagal sendiri-sendiri per grafik. dw-service mati tidak
// boleh mengosongkan kartu angka yang datangnya dari tempat lain.

export function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(Number(n ?? 0))
}

export function formatCount(n) {
  return new Intl.NumberFormat('id-ID').format(Number(n ?? 0))
}

const monthTick = (v) => String(v).slice(2, 7)

function RoleDashboard({ title, description, sections, tiles, charts }) {
  const { companyId } = useCompany()
  const [summary, setSummary] = useState(null)
  const [summaryError, setSummaryError] = useState('')
  const [loading, setLoading] = useState(true)
  const [chartRows, setChartRows] = useState({})
  const [chartErrors, setChartErrors] = useState({})

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    setLoading(true)
    setSummaryError('')
    apiClient
      .get('/api/ai-bi/dashboards/summary', { params: { company_id: companyId, sections: sections.join(',') } })
      .then(({ data }) => setSummary(data))
      .catch(() => setSummaryError('Gagal memuat ringkasan. Pastikan ai-bi-service aktif.'))
      .finally(() => setLoading(false))

    charts.forEach((chart) => {
      apiClient
        .get(`/api/dw/analytics/${chart.endpoint}`, { params: { company_id: companyId } })
        .then(({ data }) => {
          // dw-service mengirim angka desimal sebagai string (ClickHouse
          // Decimal); chart butuh number, jadi tiap field seri dikonversi di
          // satu tempat alih-alih di setiap grafik.
          const rows = data.map((row) => {
            const converted = { ...row }
            chart.series.forEach((s) => {
              converted[s.key] = Number(row[s.key] ?? 0)
            })
            return converted
          })
          setChartRows((prev) => ({ ...prev, [chart.endpoint]: rows }))
        })
        .catch(() => {
          setChartErrors((prev) => ({ ...prev, [chart.endpoint]: 'Gagal memuat data. Pastikan dw-service aktif.' }))
        })
    })
  }, [companyId, sections, charts])

  const tileList = summary ? tiles(summary) : []

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">{title}</h2>
        <div className="text-secondary small">{description}</div>
      </div>

      {!companyId && <div className="alert alert-warning py-2 small mb-0">Pilih company dulu di kanan atas.</div>}
      {summaryError && <div className="alert alert-danger py-2 small mb-0">{summaryError}</div>}
      {loading && <div className="text-secondary small">Memuat dashboard...</div>}

      {/* Service yang tidak bisa dihubungi disebut namanya, bukan disembunyikan:
          kartu yang menampilkan nol karena sumbernya mati terlihat persis sama
          dengan kartu yang menampilkan nol karena datanya memang belum ada. */}
      {summary?.errors?.length > 0 && (
        <div className="alert alert-warning py-2 small mb-0">
          Sebagian angka tidak tersedia — {summary.errors.map((e) => e.source).join(', ')} tidak bisa dihubungi.
        </div>
      )}

      {tileList.length > 0 && <div className="row g-3">{tileList.map((tile) => <StatTile key={tile.label} {...tile} />)}</div>}

      {charts.map((chart) => (
        <div key={chart.endpoint} className="card p-3">
          <div className="fw-semibold">{chart.title}</div>
          {chart.note && <div className="text-secondary small mb-2">{chart.note}</div>}
          {chartErrors[chart.endpoint] && <div className="text-danger small">{chartErrors[chart.endpoint]}</div>}
          {!chartErrors[chart.endpoint] && (
            <GroupedBarChart
              data={chartRows[chart.endpoint] ?? []}
              series={chart.series}
              formatValue={chart.format ?? formatCount}
              categoryKey={chart.categoryKey ?? 'month'}
              formatCategoryTick={chart.tick ?? monthTick}
            />
          )}
        </div>
      ))}
    </div>
  )
}

export default RoleDashboard
