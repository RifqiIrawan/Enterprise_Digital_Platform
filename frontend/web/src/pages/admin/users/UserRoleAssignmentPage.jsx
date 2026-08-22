import { useEffect, useMemo, useState } from 'react'
import apiClient from '../../../services/apiClient.js'
import Modal from '../../../components/common/Modal.jsx'
import DataTable, { TruncatedText } from '../../../components/common/DataTable.jsx'
import RolePickerDropdown from '../../../components/common/RolePickerDropdown.jsx'
import UserAccessModal from './UserAccessModal.jsx'
import { colorFor, initials } from '../../../utils/avatarColor.js'
import { useCompany } from '../../../store/CompanyContext.jsx'
import { usePagePermission } from '../../../store/PermissionContext.jsx'

const emptyUserForm = { email: '', full_name: '', phone: '', password: '' }
const emptyEditForm = { full_name: '', phone: '', status: 'active' }

const STATUS_BADGE = {
  active: 'text-bg-success',
  inactive: 'text-bg-secondary',
  locked: 'text-bg-danger',
}

function Avatar({ seed, label, size = 40 }) {
  const { bg, fg } = colorFor(seed)
  return (
    <span
      className="edp-avatar-dyn"
      style={{ width: size, height: size, background: bg, color: fg, fontSize: size * 0.38 }}
    >
      {initials(label)}
    </span>
  )
}

function formatLastLogin(value) {
  if (!value) return 'Belum pernah login'
  return new Date(value).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
}

function UserRoleAssignmentPage() {
  const { companyId } = useCompany()
  const { can } = usePagePermission()
  const [users, setUsers] = useState([])
  const [roles, setRoles] = useState([])
  const [userRolesMap, setUserRolesMap] = useState({}) // userId -> UserRoleView[]
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [activeFilter, setActiveFilter] = useState('all')

  const [managingUser, setManagingUser] = useState(null)
  const [assigning, setAssigning] = useState(false)

  const [accessUser, setAccessUser] = useState(null)
  const [resetResult, setResetResult] = useState(null)
  const [resettingId, setResettingId] = useState(null)

  const [editingUser, setEditingUser] = useState(null)
  const [editForm, setEditForm] = useState(emptyEditForm)
  const [editError, setEditError] = useState('')
  const [savingEdit, setSavingEdit] = useState(false)

  const [creatingUser, setCreatingUser] = useState(false)
  const [userForm, setUserForm] = useState(emptyUserForm)
  const [userFormError, setUserFormError] = useState('')
  const [savingUser, setSavingUser] = useState(false)

  function loadUserRoles(userId) {
    return apiClient
      .get('/api/rbac/user-roles', { params: { user_id: userId } })
      .then(({ data }) => data)
      .catch(() => [])
  }

  function loadAll() {
    setLoading(true)
    Promise.all([
      apiClient.get('/api/auth/users'),
      apiClient.get('/api/rbac/roles'),
    ])
      .then(async ([usersRes, rolesRes]) => {
        setUsers(usersRes.data)
        setRoles(rolesRes.data)

        const entries = await Promise.all(
          usersRes.data.map(async (u) => [u.id, await loadUserRoles(u.id)])
        )
        setUserRolesMap(Object.fromEntries(entries))
      })
      .catch(() => setError('Gagal memuat data. Pastikan auth-service dan rbac-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(loadAll, [])

  async function refreshUserRoles(userId) {
    const data = await loadUserRoles(userId)
    setUserRolesMap((prev) => ({ ...prev, [userId]: data }))
    return data
  }

  // Hanya role yang benar-benar dipakai minimal satu user yang jadi tab,
  // supaya tidak menampilkan belasan tab kosong dari seluruh role sistem.
  const roleTabs = useMemo(() => {
    const counts = {}
    Object.values(userRolesMap).forEach((assignments) => {
      assignments.forEach((a) => {
        counts[a.role_id] = (counts[a.role_id] ?? 0) + 1
      })
    })
    return roles
      .filter((r) => counts[r.id] > 0)
      .map((r) => ({ id: r.id, label: r.name, count: counts[r.id] }))
      .sort((a, b) => b.count - a.count)
  }, [roles, userRolesMap])

  const filteredUsers = useMemo(() => {
    if (activeFilter === 'all') return users
    return users.filter((u) => (userRolesMap[u.id] ?? []).some((a) => a.role_id === activeFilter))
  }, [users, userRolesMap, activeFilter])

  async function handleAssign(roleIds) {
    if (roleIds.length === 0 || !companyId || !managingUser) return
    setAssigning(true)
    try {
      const results = await Promise.allSettled(
        roleIds.map((roleId) =>
          apiClient.post('/api/rbac/user-roles', {
            user_id: managingUser.id,
            role_id: roleId,
            company_id: companyId,
          })
        )
      )
      const failed = results.filter((r) => r.status === 'rejected')
      if (failed.length > 0) {
        window.alert(`${failed.length} dari ${roleIds.length} role gagal ditugaskan.`)
      }
      refreshUserRoles(managingUser.id)
    } finally {
      setAssigning(false)
    }
  }

  async function handleRevoke(userRoleId) {
    if (!managingUser || !window.confirm('Cabut role ini dari user?')) return
    try {
      await apiClient.delete(`/api/rbac/user-roles/${userRoleId}`)
      refreshUserRoles(managingUser.id)
    } catch {
      window.alert('Gagal mencabut role')
    }
  }

  // Password sementara hanya bisa dibaca SEKALI (yang tersimpan cuma hash-nya),
  // jadi hasilnya ditampilkan di modal sampai admin menutupnya sendiri.
  async function handleResetPassword(u) {
    if (!window.confirm(`Reset password ${u.full_name}? Sesi yang sedang berjalan akan diputus.`)) return
    setResettingId(u.id)
    try {
      const { data } = await apiClient.post(`/api/auth/users/${u.id}/reset-password`, {})
      setResetResult({ user: u, password: data.temporary_password })
      loadAll()
    } catch (err) {
      window.alert(err.response?.data?.error ?? 'Gagal mereset password')
    } finally {
      setResettingId(null)
    }
  }

  function openEditUser(u) {
    setEditForm({ full_name: u.full_name, phone: u.phone ?? '', status: u.status })
    setEditError('')
    setEditingUser(u)
  }

  async function handleUpdateUser(e) {
    e.preventDefault()
    setSavingEdit(true)
    setEditError('')
    try {
      await apiClient.put(`/api/auth/users/${editingUser.id}`, editForm)
      setEditingUser(null)
      loadAll()
    } catch (err) {
      setEditError(err.response?.data?.error ?? 'Gagal memperbarui user')
    } finally {
      setSavingEdit(false)
    }
  }

  async function handleCreateUser(e) {
    e.preventDefault()
    setSavingUser(true)
    setUserFormError('')
    try {
      await apiClient.post('/api/auth/users', userForm)
      setCreatingUser(false)
      setUserForm(emptyUserForm)
      loadAll()
    } catch (err) {
      setUserFormError(err.response?.data?.error ?? 'Gagal membuat user')
    } finally {
      setSavingUser(false)
    }
  }

  const managingUserRoles = managingUser ? userRolesMap[managingUser.id] ?? [] : []
  const assignableRoles = roles.filter((r) => !managingUserRoles.some((ur) => ur.role_id === r.id))

  const userColumns = [
    {
      key: 'full_name',
      label: 'Account',
      maxWidth: 260,
      render: (u) => (
        <div className="d-flex align-items-center gap-2" style={{ minWidth: 0 }}>
          <Avatar seed={u.id} label={u.full_name} size={34} />
          <TruncatedText value={u.full_name} maxWidth={170} className="fw-medium" />
          {u.is_super_admin && <span className="badge text-bg-warning">Super Admin</span>}
        </div>
      ),
    },
    {
      key: 'email',
      label: 'Email',
      maxWidth: 220,
      cellClassName: 'text-secondary',
      render: (u) => <TruncatedText value={u.email} maxWidth={220} />,
    },
    {
      key: 'roles',
      label: 'Role',
      sortable: false,
      render: (u) => {
        const assignments = userRolesMap[u.id] ?? []
        return (
          <div className="d-flex flex-wrap gap-1">
            {assignments.length === 0 && <span className="text-secondary small fst-italic">&mdash;</span>}
            {assignments.map((a) => {
              const { bg, fg } = colorFor(a.role_code)
              return (
                <span key={a.id} className="edp-role-chip edp-role-chip-sm" style={{ background: bg, color: fg }}>
                  {a.role_name}
                </span>
              )
            })}
          </div>
        )
      },
    },
    {
      key: 'status',
      label: 'Status',
      render: (u) => <span className={`badge ${STATUS_BADGE[u.status] ?? 'text-bg-secondary'}`}>{u.status}</span>,
    },
    {
      key: 'last_login_at',
      label: 'Login Terakhir',
      cellClassName: 'text-secondary small',
      render: (u) => formatLastLogin(u.last_login_at),
    },
    {
      key: 'actions',
      label: 'Aksi',
      sortable: false,
      className: 'text-end',
      cellClassName: 'text-end',
      render: (u) => (
        <div className="d-flex gap-1 justify-content-end">
          <button type="button" className="btn btn-sm btn-outline-secondary" onClick={() => openEditUser(u)}>
            <i className="bi bi-pencil me-1" />
            Edit
          </button>
          <button type="button" className="btn btn-sm btn-outline-primary" onClick={() => setManagingUser(u)}>
            <i className="bi bi-shield-check me-1" />
            Kelola Role
          </button>
          {can('update') && (
            <button
              type="button"
              className="btn btn-sm btn-outline-warning"
              disabled={resettingId === u.id}
              onClick={() => handleResetPassword(u)}
              title="Tetapkan password sementara & putus semua sesi"
            >
              <i className="bi bi-key me-1" />
              Reset Password
            </button>
          )}
          <button
            type="button"
            className="btn btn-sm btn-outline-primary"
            onClick={() => setAccessUser(u)}
            disabled={!companyId}
            title={companyId ? 'Hak akses per menu' : 'Pilih company dulu'}
          >
            <i className="bi bi-sliders me-1" />
            Hak Akses
          </button>
        </div>
      ),
    },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">User Management</h2>
          <div className="edp-page-subtitle">Kelola akun user beserta role yang ditugaskan ke masing-masing.</div>
        </div>
        {can('create') && (
          <button type="button" className="btn btn-primary btn-sm" onClick={() => setCreatingUser(true)}>
            <i className="bi bi-plus-lg me-1" />
            Tambah User
          </button>
        )}
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}
      {!loading && !companyId && (
        <div className="alert alert-warning py-2 small">
          Belum ada company terdaftar &mdash; role tidak bisa ditugaskan sebelum ada company.
        </div>
      )}

      <div className="card p-3">
        <ul className="nav edp-filter-tabs mb-2">
          <li className="nav-item">
            <button
              type="button"
              className={`edp-filter-tab ${activeFilter === 'all' ? 'active' : ''}`}
              onClick={() => setActiveFilter('all')}
            >
              Semua <span className="text-body-tertiary">({users.length})</span>
            </button>
          </li>
          {roleTabs.map((tab) => (
            <li className="nav-item" key={tab.id}>
              <button
                type="button"
                className={`edp-filter-tab ${activeFilter === tab.id ? 'active' : ''}`}
                onClick={() => setActiveFilter(tab.id)}
              >
                {tab.label} <span className="text-body-tertiary">({tab.count})</span>
              </button>
            </li>
          ))}
        </ul>

        <DataTable
          columns={userColumns}
          data={filteredUsers}
          loading={loading}
          searchPlaceholder="Cari nama atau email..."
          emptyMessage="Tidak ada user yang cocok."
        />
      </div>

      {managingUser && (
        <Modal title="Kelola Role" onClose={() => setManagingUser(null)}>
          <div className="d-flex flex-column gap-3">
            <div className="d-flex align-items-center gap-3">
              <Avatar seed={managingUser.id} label={managingUser.full_name} size={48} />
              <div>
                <div className="fw-semibold">{managingUser.full_name}</div>
                <div className="small text-secondary">{managingUser.email}</div>
              </div>
            </div>

            <div>
              <div className="small text-secondary mb-2">
                Role aktif <span className="text-body-tertiary">({managingUserRoles.length})</span>
              </div>
              <div className="d-flex flex-wrap gap-2">
                {managingUserRoles.length === 0 && (
                  <span className="small text-secondary fst-italic">Belum ada role ditugaskan.</span>
                )}
                {managingUserRoles.map((ur) => {
                  const { bg, fg } = colorFor(ur.role_code)
                  return (
                    <span key={ur.id} className="edp-role-chip" style={{ background: bg, color: fg }}>
                      {ur.role_name}
                      {can('delete') && (
                        <button
                          type="button"
                          className="edp-role-chip-remove"
                          style={{ color: fg }}
                          onClick={() => handleRevoke(ur.id)}
                          aria-label={`Cabut role ${ur.role_name}`}
                        >
                          <i className="bi bi-x" />
                        </button>
                      )}
                    </span>
                  )
                })}
              </div>
            </div>

            <div>
              <RolePickerDropdown
                key={managingUser.id}
                roles={assignableRoles}
                disabled={!companyId || assignableRoles.length === 0}
                onAssign={handleAssign}
                assigning={assigning}
              />
            </div>
          </div>
        </Modal>
      )}

      {resetResult && (
        <Modal
          title="Password Sementara"
          onClose={() => setResetResult(null)}
          footer={
            <button type="button" className="btn btn-primary" onClick={() => setResetResult(null)}>
              Sudah dicatat
            </button>
          }
        >
          <div className="d-flex flex-column gap-3">
            <div>
              Password sementara untuk <strong>{resetResult.user.full_name}</strong> ({resetResult.user.email}):
            </div>
            <div className="bg-body-secondary border rounded p-3 text-center">
              <code className="fs-5">{resetResult.password}</code>
            </div>
            <div className="alert alert-warning py-2 small mb-0">
              Nilai ini <strong>tidak bisa dilihat lagi</strong> setelah modal ditutup &mdash; yang tersimpan
              hanya hash-nya. Sampaikan langsung ke yang bersangkutan; dia akan diminta menggantinya sendiri
              saat login berikutnya, dan seluruh sesi lamanya sudah diputus.
            </div>
          </div>
        </Modal>
      )}

      {accessUser && (
        <UserAccessModal user={accessUser} companyId={companyId} onClose={() => setAccessUser(null)} />
      )}

      {editingUser && (
        <Modal
          title="Edit User"
          onClose={() => setEditingUser(null)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setEditingUser(null)}>
                Batal
              </button>
              <button type="submit" form="user-edit-form" className="btn btn-primary" disabled={savingEdit}>
                {savingEdit ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="user-edit-form" onSubmit={handleUpdateUser} className="d-flex flex-column gap-3">
            {editError && <div className="alert alert-danger py-2 small mb-0">{editError}</div>}
            <div className="d-flex align-items-center gap-3">
              <Avatar seed={editingUser.id} label={editingUser.full_name} size={44} />
              <div>
                <div className="fw-semibold">{editingUser.email}</div>
                <div className="small text-secondary">
                  Email &amp; username tidak bisa diubah &mdash; keduanya identitas login.
                </div>
              </div>
            </div>
            <div>
              <label className="form-label">Nama Lengkap</label>
              <input
                type="text"
                className="form-control"
                value={editForm.full_name}
                onChange={(e) => setEditForm({ ...editForm, full_name: e.target.value })}
                required
              />
            </div>
            <div>
              <label className="form-label">Telepon</label>
              <input
                type="text"
                className="form-control"
                value={editForm.phone}
                onChange={(e) => setEditForm({ ...editForm, phone: e.target.value })}
              />
            </div>
            <div>
              <label className="form-label">Status</label>
              <select
                className="form-select"
                value={editForm.status}
                onChange={(e) => setEditForm({ ...editForm, status: e.target.value })}
              >
                <option value="active">active</option>
                <option value="inactive">inactive</option>
                <option value="locked">locked</option>
              </select>
              <div className="form-text">
                Selain <code>active</code>, user tidak bisa login dan seluruh sesinya langsung dicabut.
              </div>
            </div>
          </form>
        </Modal>
      )}

      {creatingUser && (
        <Modal
          title="Tambah User"
          onClose={() => setCreatingUser(false)}
          footer={
            <>
              <button type="button" className="btn btn-outline-secondary" onClick={() => setCreatingUser(false)}>
                Batal
              </button>
              <button type="submit" form="user-form" className="btn btn-primary" disabled={savingUser}>
                {savingUser ? 'Menyimpan...' : 'Simpan'}
              </button>
            </>
          }
        >
          <form id="user-form" onSubmit={handleCreateUser} className="d-flex flex-column gap-3">
            {userFormError && <div className="alert alert-danger py-2 small mb-0">{userFormError}</div>}
            <div>
              <label className="form-label">Nama Lengkap</label>
              <input
                type="text"
                className="form-control"
                value={userForm.full_name}
                onChange={(e) => setUserForm({ ...userForm, full_name: e.target.value })}
                required
              />
            </div>
            <div>
              <label className="form-label">Email</label>
              <input
                type="email"
                className="form-control"
                value={userForm.email}
                onChange={(e) => setUserForm({ ...userForm, email: e.target.value })}
                required
              />
            </div>
            <div>
              <label className="form-label">Telepon</label>
              <input
                type="text"
                className="form-control"
                value={userForm.phone}
                onChange={(e) => setUserForm({ ...userForm, phone: e.target.value })}
              />
            </div>
            <div>
              <label className="form-label">Password</label>
              <input
                type="password"
                className="form-control"
                minLength={8}
                value={userForm.password}
                onChange={(e) => setUserForm({ ...userForm, password: e.target.value })}
                required
              />
              <div className="form-text">Minimal 8 karakter.</div>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

export default UserRoleAssignmentPage
