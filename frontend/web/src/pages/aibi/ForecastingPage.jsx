import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import ForecastLineChart from './ForecastLineChart.jsx'

function formatMoney(n) {
  return `Rp ${new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)}`
}

function formatQty(n) {
  return `${new Intl.NumberFormat('id-ID', { minimumFractionDigits: 0 }).format(n ?? 0)} pcs`
}

function ForecastingPage() {
  const [companyId, setCompanyId] = useState('')
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  function loadForecast(cid) {
    setLoading(true)
    apiClient
      .get('/api/ai-bi/forecasting/summary', { params: { company_id: cid } })
      .then(({ data }) => setData(data))
      .catch(() => setError('Gagal memuat forecasting. Pastikan ai-bi-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) loadForecast(cid)
        else setLoading(false)
      })
      .catch(() => {
        setError('Gagal memuat data company.')
        setLoading(false)
      })
  }, [])

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">Forecasting</h2>
        <div className="text-secondary small">
          Proyeksi tren sederhana (regresi linear atas histori bulanan) — bukan model machine learning, cuma sinyal arah tren.
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {loading && <div className="text-secondary small">Memuat...</div>}

      {data && (
        <>
          {data.errors?.length > 0 && (
            <div className="alert alert-warning py-2 small mb-0">
              Sebagian data gagal dimuat: {data.errors.map((e) => e.source).join(', ')}.
            </div>
          )}

          <div className="row g-3">
            <div className="col-lg-6">
              <div className="card p-3">
                <h6 className="mb-3">Revenue Sales per Bulan</h6>
                <ForecastLineChart history={data.sales_revenue.history} forecast={data.sales_revenue.forecast} formatValue={formatMoney} />
              </div>
            </div>
            <div className="col-lg-6">
              <div className="card p-3">
                <h6 className="mb-3">Total Stok (Semua Gudang) per Bulan</h6>
                <ForecastLineChart history={data.stock_level.history} forecast={data.stock_level.forecast} formatValue={formatQty} color="#0d9488" />
              </div>
            </div>
          </div>

          <div className="text-secondary small">Terakhir dimuat: {new Date(data.generated_at).toLocaleString('id-ID')}</div>
        </>
      )}
    </div>
  )
}

export default ForecastingPage
