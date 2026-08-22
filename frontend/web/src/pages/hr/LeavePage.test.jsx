import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import LeavePage from './LeavePage.jsx'
import { PermissionProvider } from '../../store/PermissionContext.jsx'
import apiClient from '../../services/apiClient.js'
import { setSession, clearSession } from '../../utils/auth.js'

// Canary untuk gating tombol di halaman sungguhan: pola yang sama dipasang di
// 50 halaman lewat skrip, jadi minimal satu halaman nyata harus membuktikan
// bahwa tombolnya benar-benar ikut hilang/muncul -- bukan hanya hook-nya yang
// mengembalikan nilai benar.
vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

function permissionRow(actions) {
  return {
    menu_id: 'menu-leave',
    menu_name: 'Cuti',
    menu_path: '/hr/leave',
    module_id: 'mod-hr',
    module_name: 'HR',
    can_view: true,
    can_create: false,
    can_update: false,
    can_delete: false,
    can_approve: false,
    can_export: false,
    source: 'override',
    role_actions: {},
    ...actions,
  }
}

function mockApi(permissions) {
  apiClient.get.mockImplementation((url) => {
    if (url.includes('/user-permissions')) return Promise.resolve({ data: [permissions] })
    // Daftar cuti & karyawan: kosong saja, yang diuji di sini tombolnya.
    return Promise.resolve({ data: [] })
  })
}

function renderLeavePage() {
  return render(
    <MemoryRouter initialEntries={['/hr/leave']}>
      <PermissionProvider>
        <LeavePage />
      </PermissionProvider>
    </MemoryRouter>
  )
}

beforeEach(() => {
  clearSession()
  setSession('token-uji', { id: 'user-1', email: 'u@edp.test', full_name: 'U', is_super_admin: false })
  apiClient.get.mockReset()
})

afterEach(() => {
  cleanup()
  clearSession()
})

describe('LeavePage — gating tombol', () => {
  it('menampilkan "Ajukan Cuti" saat user punya hak create', async () => {
    mockApi(permissionRow({ can_create: true }))

    renderLeavePage()

    expect(await screen.findByRole('button', { name: /Ajukan Cuti/i })).toBeTruthy()
  })

  it('menyembunyikan "Ajukan Cuti" saat hak create dicabut', async () => {
    mockApi(permissionRow({ can_create: false }))

    renderLeavePage()

    // Tunggu sampai hak selesai dimuat (sebelum itu default-nya "boleh"),
    // baru pastikan tombolnya benar-benar tidak ada.
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith(
        '/api/rbac/user-permissions',
        expect.objectContaining({ params: expect.objectContaining({ user_id: 'user-1' }) })
      )
    )
    await waitFor(() => expect(screen.queryByRole('button', { name: /Ajukan Cuti/i })).toBeNull())
    // Halamannya sendiri tetap terbuka -- yang dicabut hanya aksinya.
    expect(screen.getByText('Cuti')).toBeTruthy()
  })
})
