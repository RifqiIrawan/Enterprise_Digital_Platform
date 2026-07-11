import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import StatTile from '../../components/dashboard/StatTile.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

function formatMoney(n) {
  return new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(n ?? 0)
}

function ArApPage() {
  const { companyId } = useCompany()
  const [summary, setSummary] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    setLoading(true)
    apiClient
      .get('/api/finance/ar-ap-summary', { params: { company_id: companyId } })
      .then(({ data }) => setSummary(data))
      .catch(() => setError('Gagal memuat ringkasan AR/AP. Pastikan finance-service aktif.'))
      .finally(() => setLoading(false))
  }, [companyId])

  const ar = summary.find((s) => s.invoice_type === 'AR')
  const ap = summary.find((s) => s.invoice_type === 'AP')

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">Accounts Payable / Receivable</h2>
        <div className="text-secondary small">Ringkasan piutang &amp; hutang outstanding dari invoice yang sudah diposting.</div>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {!loading && !error && summary.length === 0 && (
        <div className="alert alert-secondary py-2 small">
          Belum ada invoice yang diposting. Buat &amp; posting invoice di halaman Invoices terlebih dahulu.
        </div>
      )}

      <div className="row g-3">
        <StatTile
          icon="bi-cash-coin"
          label="Piutang Outstanding (AR)"
          value={loading ? '...' : formatMoney(ar?.outstanding_amount ?? 0)}
          hint={ar ? `${ar.count} invoice` : 'Belum ada invoice AR'}
          color="green"
        />
        <StatTile
          icon="bi-wallet2"
          label="Hutang Outstanding (AP)"
          value={loading ? '...' : formatMoney(ap?.outstanding_amount ?? 0)}
          hint={ap ? `${ap.count} invoice` : 'Belum ada invoice AP'}
          color="amber"
        />
      </div>
    </div>
  )
}

export default ArApPage
