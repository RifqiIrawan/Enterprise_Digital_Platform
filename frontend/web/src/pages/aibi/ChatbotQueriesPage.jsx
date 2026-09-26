import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import DataTable from '../../components/common/DataTable.jsx'

const OUTCOME_BADGE = {
  ANSWERED: 'text-bg-success',
  RETRIEVED_ONLY: 'text-bg-warning',
  NO_MATCH: 'text-bg-secondary',
  ERROR: 'text-bg-danger',
}

function ChatbotQueriesPage() {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    apiClient
      .get('/api/rag/queries')
      .then((res) => setItems(res.data?.items ?? res.data ?? []))
      .catch(() => setError('Gagal memuat riwayat. Pastikan rag-service aktif.'))
      .finally(() => setLoading(false))
  }, [])

  const totalTokens = items.reduce((n, q) => n + (q.input_tokens || 0) + (q.output_tokens || 0), 0)

  const columns = [
    { key: 'created_at', label: 'Waktu', render: (q) => new Date(q.created_at).toLocaleString('id-ID') },
    { key: 'question', label: 'Pertanyaan' },
    {
      key: 'outcome',
      label: 'Hasil',
      render: (q) => <span className={`badge ${OUTCOME_BADGE[q.outcome] ?? 'text-bg-secondary'}`}>{q.outcome}</span>,
    },
    { key: 'model', label: 'Model', cellClassName: 'small text-secondary' },
    {
      key: 'input_tokens',
      label: 'Token (in/out)',
      className: 'text-end',
      cellClassName: 'text-end',
      render: (q) => `${q.input_tokens} / ${q.output_tokens}`,
      sortValue: (q) => q.input_tokens + q.output_tokens,
    },
    { key: 'latency_ms', label: 'Latensi (ms)', className: 'text-end', cellClassName: 'text-end' },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">Riwayat Chatbot</h2>
        <div className="text-secondary small">
          Pertanyaan semua pengguna beserta pemakaian token. Total di daftar ini: {totalTokens.toLocaleString('id-ID')} token.
        </div>
      </div>
      {error && <div className="alert alert-danger py-2 small mb-0">{error}</div>}
      <div className="card p-3">
        <DataTable
          columns={columns}
          data={items}
          rowKey={(q) => q.id}
          loading={loading}
          searchPlaceholder="Cari pertanyaan..."
          emptyMessage="Belum ada pertanyaan."
        />
      </div>
    </div>
  )
}

export default ChatbotQueriesPage
