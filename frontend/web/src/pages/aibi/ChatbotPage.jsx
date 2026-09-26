import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import { useCompany } from '../../store/CompanyContext.jsx'
import { usePagePermission } from '../../store/PermissionContext.jsx'

const OUTCOME_BADGE = {
  ANSWERED: 'text-bg-success',
  RETRIEVED_ONLY: 'text-bg-warning',
  NO_MATCH: 'text-bg-secondary',
}

const OUTCOME_LABEL = {
  ANSWERED: 'Dijawab model',
  RETRIEVED_ONLY: 'Kutipan saja',
  NO_MATCH: 'Tidak ada bahan',
}

function ChatbotPage() {
  const { companyId } = useCompany()
  const { can } = usePagePermission()
  const [question, setQuestion] = useState('')
  const [result, setResult] = useState(null)
  const [asking, setAsking] = useState(false)
  const [error, setError] = useState('')
  const [docs, setDocs] = useState([])
  const [ingesting, setIngesting] = useState(false)
  const [ingestMsg, setIngestMsg] = useState('')

  const loadDocs = () =>
    apiClient
      .get('/api/rag/documents')
      .then((res) => setDocs(res.data?.items ?? res.data ?? []))
      .catch(() => setDocs([]))

  useEffect(() => {
    loadDocs()
  }, [])

  const ask = (e) => {
    e.preventDefault()
    const q = question.trim()
    if (!q || asking) return
    setAsking(true)
    setError('')
    apiClient
      .post('/api/rag/ask', { question: q, company_id: companyId || null })
      .then((res) => setResult(res.data))
      .catch((err) => {
        setResult(null)
        setError(err.response?.data?.error || 'Gagal bertanya. Pastikan rag-service aktif.')
      })
      .finally(() => setAsking(false))
  }

  const ingest = () => {
    setIngesting(true)
    setIngestMsg('')
    apiClient
      .post('/api/rag/ingest')
      .then((res) => {
        const d = res.data
        setIngestMsg(`Index dibaca ulang: ${d.documents} dokumen, ${d.chunks} bagian, ${d.changed} berubah.`)
        return loadDocs()
      })
      .catch((err) => setIngestMsg(err.response?.data?.error || 'Gagal membaca ulang index.'))
      .finally(() => setIngesting(false))
  }

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">Chatbot Dokumentasi</h2>
        <div className="text-secondary small">
          Tanya cara kerja platform. Jawaban disusun hanya dari dokumentasi yang sudah di-index, bukan dari data perusahaan.
        </div>
      </div>

      <form className="card p-3 d-flex flex-column gap-2" onSubmit={ask}>
        <textarea
          className="form-control"
          rows={2}
          maxLength={1000}
          placeholder='Mis. "Bagaimana OEE dihitung?"'
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
        />
        <div className="d-flex justify-content-between align-items-center">
          <span className="text-secondary small">{question.length}/1000</span>
          <button className="btn btn-primary btn-sm" type="submit" disabled={asking || !question.trim()}>
            {asking ? 'Mencari...' : 'Tanya'}
          </button>
        </div>
      </form>

      {error && <div className="alert alert-danger py-2 small mb-0">{error}</div>}

      {result && (
        <div className="card p-3 d-flex flex-column gap-2">
          <div className="d-flex align-items-center gap-2">
            <span className={`badge ${OUTCOME_BADGE[result.outcome] ?? 'text-bg-secondary'}`}>
              {OUTCOME_LABEL[result.outcome] ?? result.outcome}
            </span>
            {result.model && <span className="text-secondary small">{result.model}</span>}
            <span className="text-secondary small ms-auto">{result.latency_ms} ms</span>
          </div>
          {result.answer && <div style={{ whiteSpace: 'pre-wrap' }}>{result.answer}</div>}
          {result.note && <div className="alert alert-warning py-2 small mb-0">{result.note}</div>}
          {result.retrieval_note && <div className="text-secondary small">{result.retrieval_note}</div>}

          {result.sources?.length > 0 && (
            <div>
              <div className="fw-semibold small mb-1">Sumber ({result.sources.length})</div>
              <div className="d-flex flex-column gap-2">
                {result.sources.map((s) => (
                  <details key={s.chunk_id} className="border rounded p-2">
                    <summary className="small">
                      <strong>{s.title}</strong>
                      {s.heading && <span className="text-secondary"> &rsaquo; {s.heading}</span>}
                      <span className="text-secondary"> ({s.source_path})</span>
                    </summary>
                    <div className="small mt-2" style={{ whiteSpace: 'pre-wrap' }}>{s.content}</div>
                  </details>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      <div className="card p-3">
        <div className="d-flex align-items-center gap-2 mb-2">
          <div className="fw-semibold">Dokumen ter-index ({docs.length})</div>
          {can('create') && (
            <button className="btn btn-outline-secondary btn-sm ms-auto" onClick={ingest} disabled={ingesting}>
              {ingesting ? 'Membaca ulang...' : 'Baca ulang index'}
            </button>
          )}
        </div>
        {ingestMsg && <div className="small text-secondary mb-2">{ingestMsg}</div>}
        {docs.length === 0 ? (
          <div className="text-secondary small">Belum ada dokumen ter-index.</div>
        ) : (
          <ul className="small mb-0">
            {docs.map((d) => (
              <li key={d.id}>
                {d.title} <span className="text-secondary">&mdash; {d.chunk_count} bagian</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

export default ChatbotPage
