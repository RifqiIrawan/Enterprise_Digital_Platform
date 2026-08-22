import { useState } from 'react'
import { useNavigate, useLocation, Navigate } from 'react-router-dom'
import apiClient from '../../services/apiClient.js'
import { isAuthenticated, setSession } from '../../utils/auth.js'

// Kredensial Super Admin seed dev, diisi otomatis supaya login lokal cukup satu
// klik. `import.meta.env.DEV` hanya true saat `npm run dev` -- di build produksi
// (`npm run build`) konstanta ini ikut ter-tree-shake jadi string kosong.
const DEV_EMAIL = import.meta.env.DEV ? 'admin@edp.local' : ''
const DEV_PASSWORD = import.meta.env.DEV ? 'Admin@12345' : ''

function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const [email, setEmail] = useState(DEV_EMAIL)
  const [password, setPassword] = useState(DEV_PASSWORD)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  if (isAuthenticated()) {
    return <Navigate to={location.state?.from ?? '/'} replace />
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      const { data } = await apiClient.post('/api/auth/login', { email, password })
      setSession(data.access_token, data.user)
      // Password yang ditetapkan admin sempat diketahui orang lain, jadi user
      // diarahkan langsung ke halaman ganti password -- bukan ke halaman yang
      // tadi dia tuju.
      if (data.user?.must_change_password) {
        navigate('/profile', { replace: true })
        return
      }
      navigate(location.state?.from ?? '/', { replace: true })
    } catch (err) {
      setError(err.response?.data?.error ?? 'Gagal terhubung ke server. Coba lagi.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="edp-login-page d-flex align-items-center justify-content-center">
      <div className="edp-login-glow edp-login-glow-1" />
      <div className="edp-login-glow edp-login-glow-2" />

      <div className="card edp-login-card p-4 p-sm-5 position-relative" style={{ width: 400 }}>
        <div className="edp-login-mark mb-4">
          <i className="bi bi-hexagon-fill" />
        </div>
        <h4 className="mb-1">Enterprise Digital Platform</h4>
        <p className="text-secondary small mb-4">Masuk untuk melanjutkan ke dashboard Anda.</p>

        <form onSubmit={handleSubmit}>
          {error && (
            <div className="alert alert-danger py-2 small" role="alert">
              {error}
            </div>
          )}

          <div className="mb-3">
            <label htmlFor="email" className="form-label">
              Email
            </label>
            <input
              id="email"
              type="email"
              className="form-control"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="username"
              required
            />
          </div>

          <div className="mb-4">
            <label htmlFor="password" className="form-label">
              Password
            </label>
            <input
              id="password"
              type="password"
              className="form-control"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
            />
          </div>

          <button type="submit" className="btn btn-primary w-100" disabled={submitting}>
            {submitting ? 'Masuk...' : 'Masuk'}
          </button>
        </form>
      </div>
    </div>
  )
}

export default LoginPage
