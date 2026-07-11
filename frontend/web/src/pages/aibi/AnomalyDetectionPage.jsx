import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import DataTable from '../../components/common/DataTable.jsx'

function formatValue(anomaly) {
  if (anomaly.entity_type === 'stock_movement') {
    return `${new Intl.NumberFormat('id-ID').format(anomaly.value)} pcs`
  }
  return `Rp ${new Intl.NumberFormat('id-ID').format(anomaly.value)}`
}

const SOURCE_LABEL = {
  'sales-service': 'Sales',
  'purchasing-service': 'Purchasing',
  'warehouse-service': 'Warehouse',
}

const ENTITY_LABEL = {
  sales_order: 'Sales Order',
  purchase_order: 'Purchase Order',
  stock_movement: 'Mutasi Stok',
}

function AnomalyDetectionPage() {
  const [companyId, setCompanyId] = useState('')
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  function loadScan(cid) {
    setLoading(true)
    apiClient
      .get('/api/ai-bi/anomaly-detection/scan', { params: { company_id: cid } })
      .then(({ data }) => setData(data))
      .catch(() => setError('Gagal memuat anomaly detection. Pastikan ai-bi-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        const cid = data[0]?.id ?? ''
        setCompanyId(cid)
        if (cid) loadScan(cid)
        else setLoading(false)
      })
      .catch(() => {
        setError('Gagal memuat data company.')
        setLoading(false)
      })
  }, [])

  const columns = [
    { key: 'source', label: 'Modul', render: (a) => SOURCE_LABEL[a.source] ?? a.source },
    { key: 'entity_type', label: 'Tipe', render: (a) => ENTITY_LABEL[a.entity_type] ?? a.entity_type },
    { key: 'label', label: 'Referensi', render: (a) => <code>{a.label}</code> },
    { key: 'value', label: 'Nilai', className: 'text-end', cellClassName: 'text-end', render: (a) => formatValue(a) },
    {
      key: 'z_score',
      label: 'Z-Score',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (a) => (
        <span className={`badge ${Math.abs(a.z_score) >= 3 ? 'text-bg-danger' : 'text-bg-warning'}`}>
          {a.z_score > 0 ? '+' : ''}{a.z_score.toFixed(2)}
        </span>
      ),
      sortValue: (a) => Math.abs(a.z_score),
    },
    { key: 'reason', label: 'Alasan', cellClassName: 'text-secondary small' },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">Anomaly Detection</h2>
          <div className="text-secondary small">
            Transaksi yang nilainya jauh dari rata-rata historisnya sendiri (heuristik z-score, bukan model ML) — layak dicek manual, bukan vonis pasti salah.
          </div>
        </div>
        <button type="button" className="btn btn-outline-secondary btn-sm" disabled={!companyId || loading} onClick={() => loadScan(companyId)}>
          <i className="bi bi-arrow-clockwise me-1" />
          Scan Ulang
        </button>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {data?.errors?.length > 0 && (
        <div className="alert alert-warning py-2 small mb-0">
          Sebagian data gagal dipindai: {data.errors.map((e) => e.source).join(', ')}.
        </div>
      )}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={data?.anomalies ?? []}
          rowKey={(a) => `${a.source}-${a.entity_id}`}
          loading={loading}
          searchPlaceholder="Cari referensi atau alasan..."
          emptyMessage="Tidak ada anomali terdeteksi — semua transaksi masih dalam rentang wajar."
        />
      </div>

      {data && <div className="text-secondary small">Threshold z-score: ±{data.threshold_z} · Terakhir dipindai: {new Date(data.generated_at).toLocaleString('id-ID')}</div>}
    </div>
  )
}

export default AnomalyDetectionPage
