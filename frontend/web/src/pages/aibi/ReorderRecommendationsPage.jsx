import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const URGENCY_BADGE = { HIGH: 'text-bg-danger', MEDIUM: 'text-bg-warning', LOW: 'text-bg-secondary' }

function formatQty(n) {
  return new Intl.NumberFormat('id-ID', { maximumFractionDigits: 2 }).format(Number(n ?? 0))
}

function ReorderRecommendationsPage() {
  const { companyId } = useCompany()
  const [data, setData] = useState(null)
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
      .get('/api/ai-bi/recommendations/reorder', { params: { company_id: companyId } })
      .then((res) => setData(res.data))
      .catch(() => setError('Gagal memuat rekomendasi. Pastikan ai-bi-service aktif.'))
      .finally(() => setLoading(false))
  }, [companyId])

  const items = data?.items ?? []
  const basis = data?.window

  const columns = [
    {
      key: 'product_sku',
      label: 'Barang',
      render: (i) => (
        <div>
          <div><code>{i.product_sku}</code></div>
          <div className="text-secondary small">{i.product_name}</div>
        </div>
      ),
    },
    {
      key: 'on_hand',
      label: 'Stok',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (i) => (i.product_unit ? `${formatQty(i.on_hand)} ${i.product_unit}` : formatQty(i.on_hand)),
      sortValue: (i) => i.on_hand,
    },
    {
      key: 'daily_velocity',
      label: 'Pemakaian/hari',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (i) => formatQty(i.daily_velocity),
      sortValue: (i) => i.daily_velocity,
    },
    {
      key: 'days_of_cover',
      label: 'Cukup Untuk',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (i) => (
        <div className="d-flex align-items-center gap-2 justify-content-end">
          <span>{formatQty(i.days_of_cover)} hari</span>
          <span className={`badge ${URGENCY_BADGE[i.urgency] ?? 'text-bg-secondary'}`}>{i.urgency}</span>
        </div>
      ),
      sortValue: (i) => i.days_of_cover,
    },
    {
      key: 'suggested_reorder_qty',
      label: 'Saran Pesan',
      className: 'text-end',
      cellClassName: 'text-end fw-semibold',
      render: (i) => formatQty(i.suggested_reorder_qty),
      sortValue: (i) => i.suggested_reorder_qty,
    },
    { key: 'reason', label: 'Dasar', sortable: false, cellClassName: 'small text-secondary' },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">Rekomendasi Pemesanan</h2>
        <div className="text-secondary small">
          Barang yang akan habis lebih dulu, dihitung dari kecepatan pemakaian belakangan ini dibanding stok yang tersisa. Target persediaan 30 hari.
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small mb-0">{error}</div>}
      {data?.errors?.length > 0 && (
        <div className="alert alert-warning py-2 small mb-0">
          Sebagian sumber tidak bisa dihubungi &mdash; {data.errors.map((e) => e.source).join(', ')}.
        </div>
      )}

      {basis && (
        <div className="card p-3">
          <div className="small">
            Dasar perhitungan: <strong>{basis.movements_considered}</strong> pergerakan stok
            {basis.from && ` (${basis.from} s/d ${basis.to}, ${basis.days} hari)`}.
          </div>
          {/* Batas 200 baris di warehouse-service bisa membuat rentangnya jauh
              lebih pendek daripada riwayat sebenarnya; itu mengubah arti setiap
              angka di tabel ini, jadi tidak boleh disembunyikan. */}
          {basis.truncated && (
            <div className="text-warning small mt-1">
              Riwayat pergerakan terpotong di 200 baris terakhir, jadi rentang di atas kemungkinan lebih pendek daripada riwayat sebenarnya &mdash;
              kecepatan pemakaian bisa terlihat lebih tinggi daripada kenyataannya.
            </div>
          )}
        </div>
      )}

      <div className="card p-3">
        <DataTable
          columns={columns}
          data={items}
          rowKey={(i) => i.product_id}
          loading={loading}
          searchPlaceholder="Cari SKU atau nama barang..."
          emptyMessage="Tidak ada barang yang perlu dipesan ulang dalam 30 hari ke depan."
        />
      </div>
    </div>
  )
}

export default ReorderRecommendationsPage
