import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import StatTile from '../../components/dashboard/StatTile.jsx'
import { CORE_SERVICES } from '../../utils/modules.js'

// Awal hari ini dalam RFC3339 — audit-service hanya menerima format itu untuk
// filter `from` (lihat listAuditLogs), dan menghitung "hari ini" di sisi klien
// membuat angkanya cocok dengan jam dinding pengguna, bukan UTC server.
function startOfTodayRFC3339() {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  return d.toISOString()
}

const ACTION_ICON = {
  create: 'bi-plus-circle',
  update: 'bi-pencil',
  delete: 'bi-trash',
  login: 'bi-box-arrow-in-right',
}

function DashboardPage() {
  // Daftar modul diambil dari rbac-service, BUKAN dari konstanta di frontend.
  // Sumbernya sama dengan yang dipakai sidebar, jadi modul baru muncul di sini
  // begitu migrasi seed-nya jalan — daftar statis yang lama sempat tertinggal
  // 7 modul sekaligus dan masih menyebut dua modul yang tidak pernah ada.
  const [modules, setModules] = useState([])
  const [companyCount, setCompanyCount] = useState(null)
  const [userCount, setUserCount] = useState(null)
  const [todayCount, setTodayCount] = useState(null)
  const [recent, setRecent] = useState([])
  const [recentError, setRecentError] = useState('')

  useEffect(() => {
    apiClient
      .get('/api/rbac/modules')
      .then(({ data }) => setModules(data))
      .catch(() => setModules([]))

    apiClient
      .get('/api/company/companies')
      .then(({ data }) => setCompanyCount(data.length))
      .catch(() => setCompanyCount(null))

    apiClient
      .get('/api/auth/users')
      .then(({ data }) => setUserCount(Array.isArray(data) ? data.length : data?.users?.length ?? null))
      .catch(() => setUserCount(null))

    // Dua permintaan terpisah ke audit-service dengan tujuan berbeda: satu
    // menghitung aktivitas hari ini (dibatasi 500, batas maksimum backend),
    // satu lagi mengambil 8 baris terbaru untuk daftar di samping. Digabung
    // jadi satu permintaan akan memaksa salah satunya kompromi.
    apiClient
      .get('/api/audit/audit-logs', { params: { from: startOfTodayRFC3339(), limit: 500 } })
      .then(({ data }) => setTodayCount(data.length))
      .catch(() => setTodayCount(null))

    apiClient
      .get('/api/audit/audit-logs', { params: { limit: 8 } })
      .then(({ data }) => {
        setRecent(data)
        setRecentError('')
      })
      // 403 di sini BUKAN gangguan: gateway memang menolak user yang tidak
      // punya hak Audit Log (lihat internal/authz di api-gateway). Menyebutnya
      // "tidak bisa dihubungi" akan mengirim orang memeriksa service yang
      // sebenarnya sehat.
      .catch((err) =>
        setRecentError(
          err.response?.status === 403
            ? 'Anda tidak punya akses ke Audit Log.'
            : 'audit-service tidak bisa dihubungi.'
        )
      )
  }, [])

  const show = (v) => (v === null ? '—' : v)
  // Modul "core" adalah Administrasi, bukan modul bisnis — dihitung terpisah
  // supaya angka modul bisnis tidak ikut menggelembung satu.
  const businessModules = modules.filter((m) => m.code !== 'core')

  return (
    <div className="d-flex flex-column gap-4">
      <div className="row g-3">
        <StatTile
          icon="bi-hdd-network"
          label="Modul bisnis aktif"
          value={businessModules.length === 0 ? '—' : businessModules.length}
          hint={`plus ${CORE_SERVICES.length} service inti (gateway, auth, company, rbac, audit)`}
          color="primary"
        />
        <StatTile
          icon="bi-building"
          label="Total perusahaan"
          value={show(companyCount)}
          hint={companyCount === null ? 'company-service tidak bisa dihubungi' : 'dari company-service'}
          color="green"
        />
        <StatTile
          icon="bi-people"
          label="Total pengguna"
          value={show(userCount)}
          hint={userCount === null ? 'auth-service tidak bisa dihubungi' : 'dari auth-service'}
          color="amber"
        />
        <StatTile
          icon="bi-journal-text"
          label="Aktivitas hari ini"
          value={todayCount === null ? '—' : todayCount === 500 ? '500+' : todayCount}
          hint={todayCount === null ? 'audit-service tidak bisa dihubungi' : 'event tercatat sejak tengah malam'}
          color="blue"
        />
      </div>

      <div className="row g-3">
        <div className="col-12 col-xl-8">
          <div className="card h-100">
            <div className="card-header">
              <span className="fw-semibold">Modul terpasang</span>
            </div>
            <div className="card-body d-flex flex-wrap gap-2">
              {CORE_SERVICES.map((service) => (
                <span className="edp-module-pill is-active" key={service.key}>
                  <span className="edp-dot" />
                  {service.label}
                </span>
              ))}
              {businessModules.map((mod) => (
                <span className="edp-module-pill is-active" key={mod.id}>
                  <span className="edp-dot" />
                  {mod.name}
                </span>
              ))}
              {modules.length === 0 && (
                <span className="text-secondary small">Gagal memuat daftar modul dari rbac-service.</span>
              )}
            </div>
            {/* Legenda "belum diimplementasi" yang lama dihapus: setiap modul
                yang terdaftar di rbac-service memang sudah punya service dan
                halamannya. Kalau nanti ada modul yang di-seed lebih dulu
                sebelum dibangun, legenda itu perlu kembali. */}
            <div className="card-footer text-secondary small">
              Daftar ini berasal dari rbac-service, sumber yang sama dengan menu di samping.
            </div>
          </div>
        </div>

        <div className="col-12 col-xl-4">
          <div className="card h-100">
            <div className="card-header">
              <span className="fw-semibold">Aktivitas terbaru</span>
            </div>
            {recent.length === 0 ? (
              <div className="card-body d-flex flex-column align-items-center justify-content-center text-center text-secondary py-5">
                <i className="bi bi-inbox fs-2 mb-2" />
                <div className="small">{recentError || 'Belum ada aktivitas tercatat.'}</div>
                {!recentError && (
                  <div className="small">Audit log terisi saat Kafka aktif ketika perubahan terjadi.</div>
                )}
              </div>
            ) : (
              <ul className="list-group list-group-flush">
                {recent.map((log) => (
                  <li key={log.id} className="list-group-item d-flex gap-2 align-items-start">
                    <i className={`bi ${ACTION_ICON[log.action] ?? 'bi-dot'} text-secondary mt-1`} />
                    <div className="flex-grow-1 min-w-0">
                      <div className="small text-truncate" title={log.event_type}>
                        {log.event_type}
                      </div>
                      <div className="text-secondary" style={{ fontSize: '0.75rem' }}>
                        {log.actor_email ?? log.source_service} ·{' '}
                        {new Date(log.occurred_at).toLocaleString('id-ID', {
                          day: '2-digit',
                          month: 'short',
                          hour: '2-digit',
                          minute: '2-digit',
                        })}
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export default DashboardPage
