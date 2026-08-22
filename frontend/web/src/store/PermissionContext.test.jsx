import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { PermissionProvider, usePagePermission } from './PermissionContext.jsx'
import apiClient from '../services/apiClient.js'
import { setSession, clearSession } from '../utils/auth.js'

vi.mock('../services/apiClient.js', () => ({
  default: { get: vi.fn() },
}))

// CompanyContext di-mock supaya test ini fokus pada resolusi hak, bukan pada
// pemuatan daftar company (yang punya jalur jaringannya sendiri).
let mockCompanyId = 'company-1'
vi.mock('./CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: mockCompanyId }),
}))

function permissionRow(path, actions = {}) {
  return {
    menu_id: 'menu-' + path,
    menu_name: path,
    menu_path: path,
    module_id: 'mod',
    module_name: 'Modul',
    can_view: false,
    can_create: false,
    can_update: false,
    can_delete: false,
    can_approve: false,
    can_export: false,
    source: 'role',
    role_actions: {},
    ...actions,
  }
}

// Probe menampilkan hasil can() sebagai teks supaya assertion-nya sederhana.
function Probe() {
  const { can, loaded } = usePagePermission()
  return (
    <div>
      <span data-testid="loaded">{String(loaded)}</span>
      <span data-testid="create">{String(can('create'))}</span>
      <span data-testid="approve">{String(can('approve'))}</span>
      <span data-testid="delete">{String(can('delete'))}</span>
    </div>
  )
}

function renderAt(pathname) {
  return render(
    <MemoryRouter initialEntries={[pathname]}>
      <PermissionProvider>
        <Probe />
      </PermissionProvider>
    </MemoryRouter>
  )
}

function loginAs({ superAdmin = false } = {}) {
  setSession('token-uji', {
    id: 'user-1',
    email: 'user@edp.test',
    full_name: 'User Uji',
    is_super_admin: superAdmin,
  })
}

beforeEach(() => {
  mockCompanyId = 'company-1'
  clearSession()
  apiClient.get.mockReset()
})

afterEach(() => {
  // Auto-cleanup Testing Library hanya aktif kalau `globals` menyala di
  // Vitest; di sini globals sengaja dimatikan, jadi DOM dibersihkan manual --
  // tanpa ini render test berikutnya menumpuk di body yang sama.
  cleanup()
  clearSession()
})

describe('PermissionProvider', () => {
  it('memakai hak dari user-permissions untuk halaman yang sedang dibuka', async () => {
    loginAs()
    apiClient.get.mockResolvedValue({
      data: [
        permissionRow('/hr/leave', { can_view: true, can_create: true }),
        permissionRow('/hr/overtime', { can_view: true, can_approve: true }),
      ],
    })

    renderAt('/hr/leave')

    await waitFor(() => expect(screen.getByTestId('loaded').textContent).toBe('true'))
    expect(screen.getByTestId('create').textContent).toBe('true')
    expect(screen.getByTestId('approve').textContent).toBe('false')
  })

  it('mengirim user_id & company_id yang sedang aktif', async () => {
    loginAs()
    apiClient.get.mockResolvedValue({ data: [] })

    renderAt('/hr/leave')

    await waitFor(() => expect(apiClient.get).toHaveBeenCalled())
    expect(apiClient.get).toHaveBeenCalledWith('/api/rbac/user-permissions', {
      params: { user_id: 'user-1', company_id: 'company-1' },
    })
  })

  // Rute turunan tanpa menu sendiri (mis. /admin/roles/new) mewarisi hak menu
  // induknya -- kalau tidak, tombol Simpan di halaman itu ikut hilang.
  it('rute turunan mewarisi hak menu induknya', async () => {
    loginAs()
    apiClient.get.mockResolvedValue({
      data: [permissionRow('/admin/roles', { can_view: true, can_create: true })],
    })

    renderAt('/admin/roles/new')

    await waitFor(() => expect(screen.getByTestId('loaded').textContent).toBe('true'))
    expect(screen.getByTestId('create').textContent).toBe('true')
  })

  // Menu yang path-nya lebih panjang harus menang, bukan yang kebetulan
  // diperiksa lebih dulu.
  it('memilih menu dengan path terpanjang yang cocok', async () => {
    loginAs()
    apiClient.get.mockResolvedValue({
      data: [
        permissionRow('/hr', { can_view: true, can_create: true, can_delete: true }),
        permissionRow('/hr/employees', { can_view: true, can_create: false, can_delete: false }),
      ],
    })

    renderAt('/hr/employees')

    await waitFor(() => expect(screen.getByTestId('loaded').textContent).toBe('true'))
    expect(screen.getByTestId('create').textContent).toBe('false')
    expect(screen.getByTestId('delete').textContent).toBe('false')
  })

  it('menu lain tidak ikut mengklaim halaman yang mirip namanya', async () => {
    loginAs()
    apiClient.get.mockResolvedValue({
      data: [permissionRow('/hr/leave', { can_view: true, can_create: false })],
    })

    // /hr/leaves BUKAN turunan /hr/leave -> tidak ada menu yang cocok ->
    // jatuh ke default "boleh".
    renderAt('/hr/leaves')

    await waitFor(() => expect(screen.getByTestId('loaded').textContent).toBe('true'))
    expect(screen.getByTestId('create').textContent).toBe('true')
  })

  // Default BOLEH saat hak gagal dimuat: ini lapisan tampilan, dan menyembunyikan
  // semua tombol setiap kali rbac-service mati akan tampak seperti aplikasi rusak.
  it('mengizinkan semuanya kalau permintaan hak gagal', async () => {
    loginAs()
    apiClient.get.mockRejectedValue(new Error('rbac mati'))

    renderAt('/hr/leave')

    await waitFor(() => expect(apiClient.get).toHaveBeenCalled())
    expect(screen.getByTestId('create').textContent).toBe('true')
    expect(screen.getByTestId('approve').textContent).toBe('true')
    expect(screen.getByTestId('loaded').textContent).toBe('false')
  })

  it('mengizinkan semuanya sebelum hak selesai dimuat', async () => {
    loginAs()
    apiClient.get.mockReturnValue(new Promise(() => {})) // tidak pernah selesai

    renderAt('/hr/leave')

    expect(screen.getByTestId('create').textContent).toBe('true')
    expect(screen.getByTestId('loaded').textContent).toBe('false')
  })

  // Halaman di luar RBAC menu (mis. /profile) bukan urusan lapisan ini.
  it('mengizinkan halaman yang tidak punya menu sama sekali', async () => {
    loginAs()
    apiClient.get.mockResolvedValue({
      data: [permissionRow('/hr/leave', { can_view: true, can_create: false })],
    })

    renderAt('/profile')

    await waitFor(() => expect(screen.getByTestId('loaded').textContent).toBe('true'))
    expect(screen.getByTestId('create').textContent).toBe('true')
  })

  // Super admin melewati role & override di backend, jadi tidak ada gunanya
  // menanyakan haknya per menu.
  it('super admin boleh semuanya tanpa memanggil API', async () => {
    loginAs({ superAdmin: true })
    apiClient.get.mockResolvedValue({ data: [] })

    renderAt('/hr/leave')

    expect(screen.getByTestId('create').textContent).toBe('true')
    expect(screen.getByTestId('approve').textContent).toBe('true')
    expect(apiClient.get).not.toHaveBeenCalled()
  })

  it('tidak memanggil API sebelum company dipilih', async () => {
    loginAs()
    mockCompanyId = null
    apiClient.get.mockResolvedValue({ data: [] })

    renderAt('/hr/leave')

    expect(apiClient.get).not.toHaveBeenCalled()
    expect(screen.getByTestId('create').textContent).toBe('true')
  })

  it('aksi yang tidak dikenal dianggap salah tulis, bukan ditolak diam-diam', async () => {
    loginAs()
    apiClient.get.mockResolvedValue({ data: [] })

    function BadProbe() {
      const { can } = usePagePermission()
      return <span>{String(can('menghapus'))}</span>
    }

    expect(() =>
      render(
        <MemoryRouter initialEntries={['/hr/leave']}>
          <PermissionProvider>
            <BadProbe />
          </PermissionProvider>
        </MemoryRouter>
      )
    ).toThrow(/aksi tidak dikenal/)
  })
})
