import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import DataTable from '../../components/common/DataTable.jsx'
import StatTile from '../../components/dashboard/StatTile.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const LEVEL_BADGE = { HIGH: 'text-bg-danger', MEDIUM: 'text-bg-warning', LOW: 'text-bg-secondary' }
const SUBJECT_LABEL = { machine: 'Mesin', asset: 'Aset' }

function PredictiveMaintenancePage() {
  const { companyId } = useCompany()
  const [scan, setScan] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    setLoading(true)
    setError('')
    apiClient
      .get('/api/ai-bi/predictive-maintenance/scan', { params: { company_id: companyId } })
      .then(({ data }) => setScan(data))
      .catch(() => setError('Gagal memuat prediksi. Pastikan ai-bi-service aktif.'))
      .finally(() => setLoading(false))
  }, [companyId])

  const items = scan?.items ?? []
  const highCount = items.filter((i) => i.risk_level === 'HIGH').length
  const mediumCount = items.filter((i) => i.risk_level === 'MEDIUM').length

  const columns = [
    {
      key: 'code',
      label: 'Subjek',
      render: (i) => (
        <div>
          <div>
            <code>{i.code}</code> <span className="badge text-bg-light">{SUBJECT_LABEL[i.subject_type] ?? i.subject_type}</span>
          </div>
          <div className="text-secondary small">{i.name}</div>
        </div>
      ),
      sortValue: (i) => i.code,
    },
    {
      key: 'risk_score',
      label: 'Risiko',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (i) => (
        <div className="d-flex align-items-center gap-2 justify-content-end">
          <span className="fw-semibold">{i.risk_score}</span>
          <span className={`badge ${LEVEL_BADGE[i.risk_level] ?? 'text-bg-secondary'}`}>{i.risk_level}</span>
        </div>
      ),
      sortValue: (i) => i.risk_score,
    },
    {
      // Skor tanpa alasan hanya akan diabaikan setelah tebakan pertamanya
      // meleset, jadi faktornya ditampilkan sebaris dengan skornya -- bukan
      // disembunyikan di balik klik.
      key: 'factors',
      label: 'Kenapa',
      sortable: false,
      render: (i) => (
        <ul className="list-unstyled mb-0 small">
          {i.factors.map((f) => (
            <li key={f.code}>
              <span className="fw-semibold">+{f.contribution}</span> {f.label}
              <span className="text-secondary"> &mdash; {f.detail}</span>
            </li>
          ))}
        </ul>
      ),
    },
    { key: 'recommended_action', label: 'Saran', sortable: false, cellClassName: 'small' },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">Predictive Maintenance</h2>
        <div className="text-secondary small">
          Peringkat mesin &amp; aset yang paling perlu diperiksa, dihitung dari OEE mesin, jadwal maintenance yang terlewat, dan sisa umur manfaat aset.
          Ini heuristik yang dijelaskan &mdash; setiap skor datang dengan faktor pembentuknya.
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small mb-0">{error}</div>}
      {scan?.errors?.length > 0 && (
        <div className="alert alert-warning py-2 small mb-0">
          Sebagian sumber tidak bisa dihubungi &mdash; {scan.errors.map((e) => e.source).join(', ')}. Daftar di bawah belum lengkap.
        </div>
      )}

      {scan && (
        <div className="row g-3">
          <StatTile icon="bi-exclamation-octagon" label="Risiko Tinggi" value={highCount} hint="perlu tindakan segera" color="rose" />
          <StatTile icon="bi-exclamation-triangle" label="Risiko Menengah" value={mediumCount} hint="pantau minggu ini" color="amber" />
          <StatTile icon="bi-list-check" label="Total Ditandai" value={items.length} hint="mesin & aset dengan setidaknya satu faktor" color="blue" />
        </div>
      )}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={items}
          rowKey={(i) => i.subject_type + ':' + i.subject_id}
          loading={loading}
          searchPlaceholder="Cari kode atau nama..."
          emptyMessage="Tidak ada mesin atau aset yang perlu ditandai."
        />
      </div>
    </div>
  )
}

export default PredictiveMaintenancePage
