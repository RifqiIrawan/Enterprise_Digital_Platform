import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import apiClient from '../../services/apiClient.js'
import { clearSession, getCurrentUser } from '../../utils/auth.js'
import { colorFor, initials } from '../../utils/avatarColor.js'

const emptyForm = { current_password: '', new_password: '', confirm_password: '' }

function ProfilePage() {
  const navigate = useNavigate()
  const user = getCurrentUser()
  const [form, setForm] = useState(emptyForm)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [done, setDone] = useState(false)

  const { bg, fg } = colorFor(user?.id ?? '')

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')

    if (form.new_password !== form.confirm_password) {
      setError('Konfirmasi password baru tidak sama.')
      return
    }

    setSaving(true)
    try {
      await apiClient.post('/api/auth/change-password', {
        current_password: form.current_password,
        new_password: form.new_password,
      })
      // Backend mencabut SELURUH refresh token saat password diganti, termasuk
      // milik sesi ini. Jadi sesi lokal ikut dibersihkan dan user diminta login
      // ulang -- bukan dibiarkan memakai access token yang sudah tidak punya
      // jalan perpanjangan.
      setDone(true)
      setForm(emptyForm)
      setTimeout(() => {
        clearSession()
        navigate('/login')
      }, 1500)
    } catch (err) {
      // 401 di sini berarti "password lama salah", bukan sesi kedaluwarsa --
      // interceptor apiClient sengaja tidak mengusir ke /login untuk endpoint
      // ini supaya pesannya bisa ditampilkan di form.
      setError(err.response?.data?.error ?? 'Gagal mengganti password')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="d-flex flex-column gap-3">
      <div>
        <h2 className="edp-page-title">Profil Saya</h2>
        <div className="edp-page-subtitle">Data akun dan penggantian password.</div>
      </div>

      <div className="row g-3">
        <div className="col-12 col-lg-5">
          <div className="card p-3 h-100">
            <div className="d-flex align-items-center gap-3 mb-3">
              <span
                className="edp-avatar-dyn"
                style={{ width: 56, height: 56, background: bg, color: fg, fontSize: 21 }}
              >
                {initials(user?.full_name ?? '?')}
              </span>
              <div>
                <div className="fw-semibold">{user?.full_name}</div>
                <div className="small text-secondary">{user?.email}</div>
              </div>
            </div>
            <dl className="row mb-0 small">
              <dt className="col-5 text-secondary fw-normal">Username</dt>
              <dd className="col-7">{user?.username || '—'}</dd>
              <dt className="col-5 text-secondary fw-normal">Telepon</dt>
              <dd className="col-7">{user?.phone || '—'}</dd>
              <dt className="col-5 text-secondary fw-normal">Status</dt>
              <dd className="col-7">
                <span className="badge text-bg-success">{user?.status ?? 'active'}</span>
              </dd>
              <dt className="col-5 text-secondary fw-normal">Super Admin</dt>
              <dd className="col-7 mb-0">{user?.is_super_admin ? 'Ya' : 'Tidak'}</dd>
            </dl>
          </div>
        </div>

        <div className="col-12 col-lg-7">
          <div className="card p-3 h-100">
            <h3 className="h6 mb-3">Ganti Password</h3>

            {user?.must_change_password && (
              <div className="alert alert-warning py-2 small">
                Password Anda baru saja direset oleh admin dan sempat diketahui orang lain. Ganti sekarang
                supaya password yang berlaku hanya diketahui Anda.
              </div>
            )}

            {done && (
              <div className="alert alert-success py-2 small">
                Password berhasil diganti. Seluruh sesi dicabut &mdash; mengarahkan ke halaman login...
              </div>
            )}
            {error && <div className="alert alert-danger py-2 small">{error}</div>}

            <form onSubmit={handleSubmit} className="d-flex flex-column gap-3">
              <div>
                <label className="form-label">Password Lama</label>
                <input
                  type="password"
                  className="form-control"
                  autoComplete="current-password"
                  value={form.current_password}
                  onChange={(e) => setForm({ ...form, current_password: e.target.value })}
                  required
                />
              </div>
              <div>
                <label className="form-label">Password Baru</label>
                <input
                  type="password"
                  className="form-control"
                  autoComplete="new-password"
                  minLength={8}
                  value={form.new_password}
                  onChange={(e) => setForm({ ...form, new_password: e.target.value })}
                  required
                />
                <div className="form-text">Minimal 8 karakter dan harus berbeda dari password lama.</div>
              </div>
              <div>
                <label className="form-label">Konfirmasi Password Baru</label>
                <input
                  type="password"
                  className="form-control"
                  autoComplete="new-password"
                  minLength={8}
                  value={form.confirm_password}
                  onChange={(e) => setForm({ ...form, confirm_password: e.target.value })}
                  required
                />
              </div>
              <div>
                <button type="submit" className="btn btn-primary" disabled={saving || done}>
                  {saving ? 'Menyimpan...' : 'Ganti Password'}
                </button>
              </div>
            </form>
          </div>
        </div>
      </div>
    </div>
  )
}

export default ProfilePage
