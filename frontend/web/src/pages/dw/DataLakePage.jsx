import { useCallback, useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import GroupedBarChart from '../aibi/GroupedBarChart.jsx'
import { FINANCE_SERIES, SALES_SERIES, STOCK_SERIES, PRODUCTION_DOWNTIME_SERIES, PRODUCTION_OEE_SERIES, QC_SERIES, PRODUCTION_SERIES, ECOMMERCE_SERIES, TICKETING_SERIES, formatPercent } from '../bi/chartSeries.js'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const STATUS_LABEL = {
  MATCH: { text: 'Cocok', cls: 'bg-success' },
  MISSING_FROM_LAKE: { text: 'Kurang di lake', cls: 'bg-warning text-dark' },
  EXTRA_IN_LAKE: { text: 'Lebih di lake', cls: 'bg-warning text-dark' },
  ERROR: { text: 'Error', cls: 'bg-danger' },
}

const number = (v) => new Intl.NumberFormat('id-ID').format(v)
const rupiah = (v) => `Rp ${number(Math.round(v))}`

// Faktor OEE null = belum bisa dihitung; tetap null (bukan 0) seperti di BI Dashboards.
function oeeSeries(rows) {
  const keys = ['availability_pct', 'performance_pct', 'quality_pct', 'oee_pct']
  return rows
    .map((r) => ({ month: r.month, ...Object.fromEntries(keys.map((k) => [k, r[k] == null ? null : Number(r[k])])) }))
    .sort((a, b) => a.month.localeCompare(b.month))
}

// Gold downtime per (bulan, alasan); chart ini hanya total menit per bulan.
function sumByMonth(rows, key) {
  const byMonth = new Map()
  for (const r of rows) byMonth.set(r.month, (byMonth.get(r.month) ?? 0) + Number(r[key]))
  return [...byMonth].map(([month, v]) => ({ month, [key]: v })).sort((a, b) => a.month.localeCompare(b.month))
}

// Gold mengirim desimal sebagai string ("5000000"); chart butuh angka.
function toSeries(rows, keys) {
  return rows
    .map((r) => ({ month: r.month, ...Object.fromEntries(keys.map((k) => [k, Number(r[k])])) }))
    .sort((a, b) => a.month.localeCompare(b.month))
}

function GoldChart({ title, hint, data, series, formatValue = rupiah }) {
  return (
    <div className="card p-3">
      <div className="fw-semibold">{title}</div>
      <div className="text-secondary small mb-2">{hint}</div>
      {data === null ? (
        <div className="text-secondary small">Memuat...</div>
      ) : data.length === 0 ? (
        <div className="text-secondary small">Dataset Gold kosong untuk company ini. Jalankan build.</div>
      ) : (
        <GroupedBarChart data={data} series={series} formatValue={formatValue} />
      )}
    </div>
  )
}

function DataLakePage() {
  const { companyId } = useCompany()
  const { can } = usePagePermission()
  const [finance, setFinance] = useState(null)
  const [sales, setSales] = useState(null)
  const [inventory, setInventory] = useState(null)
  const [downtime, setDowntime] = useState(null)
  const [oee, setOee] = useState(null)
  const [qc, setQc] = useState(null)
  const [production, setProduction] = useState(null)
  const [ecommerce, setEcommerce] = useState(null)
  const [ticketing, setTicketing] = useState(null)
  const [goldError, setGoldError] = useState('')
  const [reconcile, setReconcile] = useState(null)
  const [reconcileError, setReconcileError] = useState('')
  const [building, setBuilding] = useState(false)
  const [buildResult, setBuildResult] = useState(null)

  const loadGold = useCallback(() => {
    if (!companyId) return
    setGoldError('')
    const params = { company_id: companyId }
    Promise.all([
      apiClient.get('/api/dw/lake/gold/finance-monthly', { params }),
      apiClient.get('/api/dw/lake/gold/sales-monthly', { params }),
      apiClient.get('/api/dw/lake/gold/inventory-monthly', { params }),
      apiClient.get('/api/dw/lake/gold/downtime-monthly', { params }),
      apiClient.get('/api/dw/lake/gold/oee-monthly', { params }),
      apiClient.get('/api/dw/lake/gold/qc-monthly', { params }),
      apiClient.get('/api/dw/lake/gold/production-monthly', { params }),
      apiClient.get('/api/dw/lake/gold/ecommerce-monthly', { params }),
      apiClient.get('/api/dw/lake/gold/ticketing-monthly', { params }),
    ])
      .then(([f, s, i, d, o, q, pr, ec, tk]) => {
        setFinance(toSeries(f.data, ['revenue', 'expense']))
        setSales(toSeries(s.data, ['sales_value']))
        setInventory(toSeries(i.data, ['stock_in', 'stock_out']))
        setDowntime(sumByMonth(d.data, 'minutes'))
        setOee(oeeSeries(o.data))
        setQc(toSeries(q.data, ['pass_count', 'partial_count', 'fail_count']))
        setProduction(toSeries(pr.data, ['quantity_planned', 'quantity_produced']))
        setEcommerce(toSeries(ec.data, ['revenue']))
        setTicketing(toSeries(tk.data, ['resolved_count', 'open_count']))
      })
      .catch((err) => {
        setFinance([])
        setSales([])
        setInventory([])
        setDowntime([])
        setOee([])
        setQc([])
        setProduction([])
        setEcommerce([])
        setTicketing([])
        setGoldError(
          err.response?.status === 503
            ? 'Data lake (MinIO) tidak tersedia.'
            : 'Gagal memuat Gold. Pastikan dw-service dan MinIO aktif, dan akun ini punya akses dashboard BI.'
        )
      })
  }, [companyId])

  const loadReconcile = useCallback(() => {
    setReconcileError('')
    apiClient
      .get('/api/dw/lake/reconcile')
      .then(({ data }) => setReconcile(data))
      .catch((err) =>
        setReconcileError(
          err.response?.status === 403
            ? 'Akun ini tidak punya akses ke status rekonsiliasi.'
            : 'Gagal memuat rekonsiliasi. Pastikan ClickHouse dan MinIO aktif.'
        )
      )
  }, [])

  useEffect(() => {
    loadGold()
    loadReconcile()
  }, [loadGold, loadReconcile])

  async function handleBuild() {
    setBuilding(true)
    setBuildResult(null)
    try {
      const { data } = await apiClient.post('/api/dw/lake/build')
      setBuildResult({ errors: data.errors ?? [] })
    } catch (err) {
      setBuildResult({ errors: err.response?.data?.errors ?? [err.response?.data?.error ?? 'Gagal membangun data lake'] })
    } finally {
      setBuilding(false)
      loadGold()
      loadReconcile()
    }
  }

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Data Lake — Gold</h2>
          <div className="text-secondary small">
            Agregat bulanan siap-pakai yang dihitung dari Silver (Bronze → Silver → Gold di MinIO). Angka hanya sesegar build terakhir.
          </div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" disabled={building} onClick={handleBuild}>
            <i className="bi bi-hammer me-1" />
            {building ? 'Membangun...' : 'Build Silver + Gold'}
          </button>
        )}
      </div>

      {buildResult &&
        (buildResult.errors.length === 0 ? (
          <div className="alert alert-success py-2 small mb-0">Build selesai tanpa error.</div>
        ) : (
          <div className="alert alert-warning py-2 small mb-0">
            Build selesai sebagian — Gold dihitung dari Silver yang ada, jangan dipercaya penuh:
            {buildResult.errors.map((e) => (
              <div key={e}>{e}</div>
            ))}
          </div>
        ))}

      {goldError && <div className="alert alert-danger py-2 small mb-0">{goldError}</div>}

      <GoldChart title="Revenue vs Expense per Bulan" hint="Dataset finance_monthly" data={finance} series={FINANCE_SERIES} />
      <GoldChart title="Nilai Penjualan per Bulan" hint="Dataset sales_monthly" data={sales} series={SALES_SERIES} />
      <GoldChart
        title="Stok Masuk vs Keluar per Bulan"
        hint="Dataset inventory_monthly"
        data={inventory}
        series={STOCK_SERIES}
        formatValue={number}
      />
      <GoldChart
        title="Downtime Produksi per Bulan"
        hint="Dataset downtime_monthly (total menit semua alasan)"
        data={downtime}
        series={PRODUCTION_DOWNTIME_SERIES}
        formatValue={(v) => `${number(v)} menit`}
      />
      <GoldChart
        title="OEE per Bulan"
        hint="Dataset oee_monthly (hanya run yang sudah ditutup)"
        data={oee}
        series={PRODUCTION_OEE_SERIES}
        formatValue={formatPercent}
      />
      <GoldChart
        title="Inspeksi QC per Bulan"
        hint="Dataset qc_monthly (cacah inspeksi per hasil)"
        data={qc}
        series={QC_SERIES}
        formatValue={number}
      />
      <GoldChart
        title="Rencana vs Realisasi Produksi per Bulan"
        hint="Dataset production_monthly (realisasi hanya dari work order selesai)"
        data={production}
        series={PRODUCTION_SERIES}
        formatValue={number}
      />
      <GoldChart
        title="Penjualan E-commerce per Bulan"
        hint="Dataset ecommerce_monthly (order CANCELLED tidak dihitung)"
        data={ecommerce}
        series={ECOMMERCE_SERIES}
      />
      <GoldChart
        title="Tiket per Bulan Pembuatan"
        hint="Dataset ticketing_monthly (bulan menurut UTC, sama dengan data warehouse)"
        data={ticketing}
        series={TICKETING_SERIES}
        formatValue={number}
      />

      <div className="card p-3">
        <div className="d-flex align-items-center justify-content-between mb-1">
          <div className="fw-semibold">Rekonsiliasi Lake vs ClickHouse</div>
          {reconcile && (
            <span className={`badge ${reconcile.consistent ? 'bg-success' : 'bg-warning text-dark'}`}>
              {reconcile.consistent ? 'Konsisten' : 'Ada selisih'}
            </span>
          )}
        </div>
        <div className="text-secondary small mb-2">
          Hanya membandingkan JUMLAH baris, bukan isinya. Silver yang usang (belum di-build sejak sync terakhir) juga terbaca sebagai selisih.
        </div>
        {reconcileError && <div className="alert alert-danger py-2 small mb-0">{reconcileError}</div>}
        {reconcile && (
          <div className="table-responsive">
            <table className="table table-sm align-middle mb-0">
              <thead>
                <tr>
                  <th>Fact</th>
                  <th className="text-end">Silver</th>
                  <th className="text-end">ClickHouse</th>
                  <th className="text-end">Selisih</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {reconcile.facts.map((f) => {
                  const s = STATUS_LABEL[f.status] ?? { text: f.status, cls: 'bg-secondary' }
                  return (
                    <tr key={f.fact}>
                      <td><code>{f.fact}</code></td>
                      <td className="text-end">{number(f.silver_rows)}</td>
                      <td className="text-end">{number(f.clickhouse_rows)}</td>
                      <td className="text-end">{number(f.difference)}</td>
                      <td>
                        <span className={`badge ${s.cls}`}>{s.text}</span>
                        {f.error && <span className="text-danger small ms-2">{f.error}</span>}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

export default DataLakePage
