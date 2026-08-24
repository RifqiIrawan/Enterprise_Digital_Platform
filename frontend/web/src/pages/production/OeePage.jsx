import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import StatTile from '../../components/dashboard/StatTile.jsx'
// Komponen chart yang sama dengan yang dipakai BI Dashboards -- generik (N seri
// per kategori, satu sumbu), dan seluruh seri di sini bersatuan sama (persen),
// jadi tidak ada alasan menulis SVG kedua yang perilakunya sedikit berbeda.
import GroupedBarChart from '../aibi/GroupedBarChart.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const REASON_LABEL = {
  BREAKDOWN: 'Kerusakan mesin',
  SETUP: 'Setup / ganti cetakan',
  MATERIAL_SHORTAGE: 'Bahan baku habis',
  NO_OPERATOR: 'Tidak ada operator',
  ADJUSTMENT: 'Penyetelan',
  PLANNED_STOP: 'Berhenti terencana',
  OTHER: 'Lainnya',
}

// Availability, Performance, dan Quality adalah tiga faktor yang BERBEDA jenis
// (waktu, kecepatan, mutu), bukan dua sisi berlawanan dan bukan bagian satu
// sama lain -- jadi tiga hue kategoris, bukan satu hue beropacity seperti
// pipeline CRM. Hijau/merah sengaja dihindari, sama seperti di BI Dashboards:
// 70% Availability bukan "baik" atau "buruk" tanpa target pembanding, dan
// mewarnainya begitu adalah penilaian yang tidak dilakukan datanya.
const OEE_SERIES = [
  { key: 'availability', label: 'Availability', color: 'var(--bs-primary)' },
  { key: 'performance', label: 'Performance', color: 'var(--bs-orange)' },
  { key: 'quality', label: 'Quality', color: 'var(--bs-purple)' },
]

const percent = (v) => `${(Number(v) * 100).toFixed(1)}%`

function firstDayOfMonth() {
  const now = new Date()
  return new Date(now.getFullYear(), now.getMonth(), 1).toISOString().slice(0, 10)
}

function OeePage() {
  const { companyId, branchId } = useCompany()
  const [summary, setSummary] = useState(null)
  const [machines, setMachines] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [filter, setFilter] = useState({ from: firstDayOfMonth(), to: new Date().toISOString().slice(0, 10), machine_id: '' })

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    setLoading(true)
    setError('')
    apiClient
      .get('/api/production/oee', {
        params: {
          company_id: companyId,
          branch_id: branchId,
          from: filter.from || undefined,
          to: filter.to || undefined,
          machine_id: filter.machine_id || undefined,
        },
      })
      .then(({ data }) => setSummary(data))
      .catch(() => setError('Gagal memuat OEE. Pastikan production-service aktif.'))
      .finally(() => setLoading(false))
  }, [companyId, branchId, filter])

  useEffect(() => {
    if (!companyId) return
    apiClient.get('/api/production/machines', { params: { company_id: companyId } }).then(({ data }) => setMachines(data))
  }, [companyId])

  const chartData = (summary?.machines ?? []).map((m) => ({
    machine: m.machine_code,
    availability: m.availability,
    performance: m.performance,
    quality: m.quality,
  }))

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">OEE Mesin</h2>
        <div className="text-secondary small">
          Overall Equipment Effectiveness = Availability × Performance × Quality, dihitung dari eksekusi produksi yang sudah ditutup. Run yang masih berjalan tidak ikut.
        </div>
      </div>

      <div className="card p-3">
        <div className="row g-2 align-items-end">
          <div className="col-6 col-md-3">
            <label className="form-label small">Dari</label>
            <input type="date" className="form-control form-control-sm" value={filter.from} onChange={(e) => setFilter({ ...filter, from: e.target.value })} />
          </div>
          <div className="col-6 col-md-3">
            <label className="form-label small">Sampai</label>
            <input type="date" className="form-control form-control-sm" value={filter.to} onChange={(e) => setFilter({ ...filter, to: e.target.value })} />
          </div>
          <div className="col-12 col-md-4">
            <label className="form-label small">Mesin</label>
            <select className="form-select form-select-sm" value={filter.machine_id} onChange={(e) => setFilter({ ...filter, machine_id: e.target.value })}>
              <option value="">Semua mesin</option>
              {machines.map((m) => (
                <option key={m.id} value={m.id}>{m.code} - {m.name}</option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small mb-0">{error}</div>}
      {loading && <div className="text-secondary small">Menghitung OEE...</div>}

      {!loading && !error && summary && (
        <>
          <div className="row g-3">
            <StatTile icon="bi-speedometer2" label="OEE" value={percent(summary.overall.oee)} hint={`${summary.machines.length} mesin dengan run tertutup`} />
            <StatTile icon="bi-clock" label="Availability" value={percent(summary.overall.availability)} hint={`${summary.overall.downtime_minutes} menit downtime`} color="blue" />
            <StatTile icon="bi-lightning-charge" label="Performance" value={percent(summary.overall.performance)} hint={`${summary.overall.run_time_minutes} menit waktu jalan`} color="amber" />
            <StatTile
              icon="bi-patch-check"
              label="Quality"
              value={percent(summary.overall.quality)}
              hint={`${Number(summary.overall.quantity_good)} bagus dari ${Number(summary.overall.quantity_good) + Number(summary.overall.quantity_reject)} unit`}
              color="violet"
            />
          </div>

          {summary.overall.performance_capped && (
            <div className="alert alert-warning py-2 small mb-0">
              Performance dipotong ke 100% pada sebagian perhitungan. Cycle time ideal mesin kemungkinan disetel terlalu lambat, atau ada downtime yang belum dicatat.
            </div>
          )}

          <div className="card p-3">
            <div className="fw-semibold mb-1">Tiga Faktor per Mesin</div>
            <div className="text-secondary small mb-2">
              OEE sendiri tidak digambar sebagai batang keempat: nilainya hasil perkalian ketiganya, bukan faktor sejajar. Angkanya ada di tabel bawah.
            </div>
            <GroupedBarChart data={chartData} series={OEE_SERIES} formatValue={percent} categoryKey="machine" formatCategoryTick={(v) => v} />
          </div>

          <div className="card p-3">
            <div className="fw-semibold mb-2">Rincian per Mesin</div>
            <div className="table-responsive">
              <table className="table table-sm align-middle mb-0">
                <thead>
                  <tr>
                    <th>Mesin</th>
                    <th className="text-end">Run</th>
                    <th className="text-end">Waktu Rencana</th>
                    <th className="text-end">Downtime</th>
                    <th className="text-end">Availability</th>
                    <th className="text-end">Performance</th>
                    <th className="text-end">Quality</th>
                    <th className="text-end">OEE</th>
                  </tr>
                </thead>
                <tbody>
                  {summary.machines.length === 0 && (
                    <tr>
                      <td colSpan={8} className="text-secondary small">
                        Belum ada eksekusi produksi yang ditutup pada periode ini.
                      </td>
                    </tr>
                  )}
                  {summary.machines.map((m) => (
                    <tr key={m.machine_id}>
                      <td>
                        <code>{m.machine_code}</code> <span className="text-secondary small">{m.machine_name}</span>
                      </td>
                      <td className="text-end">{m.run_count}</td>
                      <td className="text-end">{m.planned_minutes} mnt</td>
                      <td className="text-end">{m.downtime_minutes} mnt</td>
                      <td className="text-end">{percent(m.availability)}</td>
                      <td className="text-end">
                        {percent(m.performance)}
                        {m.performance_capped && <i className="bi bi-exclamation-triangle ms-1 text-warning" title="Dipotong ke 100%" />}
                      </td>
                      <td className="text-end">{percent(m.quality)}</td>
                      <td className="text-end fw-semibold">{percent(m.oee)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div className="card p-3">
            <div className="fw-semibold mb-2">Penyebab Downtime</div>
            {summary.downtime_by_reason.length === 0 && <div className="text-secondary small">Tidak ada downtime tercatat pada periode ini.</div>}
            {summary.downtime_by_reason.map((d) => (
              <div key={d.reason_code} className="mb-2">
                <div className="d-flex justify-content-between small">
                  <span>{REASON_LABEL[d.reason_code] ?? d.reason_code}</span>
                  <span className="text-secondary">
                    {d.minutes} menit · {percent(d.share)}
                  </span>
                </div>
                <div className="progress" style={{ height: 6 }}>
                  <div className="progress-bar" style={{ width: `${Number(d.share) * 100}%` }} />
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  )
}

export default OeePage
