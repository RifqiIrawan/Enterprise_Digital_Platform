import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import PayrollPage from './PayrollPage.jsx'
import { PermissionProvider } from '../../store/PermissionContext.jsx'
import apiClient from '../../services/apiClient.js'
import { setSession, clearSession } from '../../utils/auth.js'

// Canary untuk aksi yang MEMBUKUKAN. LeavePage.test.jsx sudah membuktikan
// tombol "tambah" ikut gating create; yang dijaga di sini beda dan lebih
// penting: tombol yang mengirim angka ke buku besar harus butuh `approve`,
// bukan `create`. Gateway menuntut hak yang sama untuk
// POST /api/hr/payroll-runs/{id}/post (lihat internal/authz/policy.go), jadi
// kalau tombolnya cuma butuh create, UI akan menawarkan tindakan yang pasti
// ditolak backend.
vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

const draftRun = {
  id: 'run-1',
  period: '2026-08',
  total_employees: 3,
  total_gross: 30000000,
  total_deduction: 3000000,
  total_net: 27000000,
  status: 'DRAFT',
}

function permissionRow(actions) {
  return {
    menu_id: 'menu-payroll',
    menu_name: 'Payroll',
    menu_path: '/hr/payroll',
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
    if (url.includes('/payroll-runs')) return Promise.resolve({ data: [draftRun] })
    return Promise.resolve({ data: [] })
  })
}

function renderPayrollPage() {
  return render(
    <MemoryRouter initialEntries={['/hr/payroll']}>
      <PermissionProvider>
        <PayrollPage />
      </PermissionProvider>
    </MemoryRouter>
  )
}

async function waitForPermissionsLoaded() {
  await waitFor(() =>
    expect(apiClient.get).toHaveBeenCalledWith(
      '/api/rbac/user-permissions',
      expect.objectContaining({ params: expect.objectContaining({ user_id: 'user-1' }) })
    )
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

describe('PayrollPage — gating tombol posting', () => {
  it('menampilkan "Post ke GL" saat user punya hak approve', async () => {
    mockApi(permissionRow({ can_approve: true }))

    renderPayrollPage()

    expect(await screen.findByRole('button', { name: /Post ke GL/i })).toBeTruthy()
  })

  it('menyembunyikan "Post ke GL" saat user hanya boleh membuat payroll run', async () => {
    mockApi(permissionRow({ can_create: true }))

    renderPayrollPage()

    await waitForPermissionsLoaded()
    // Barisnya tetap ada dan tetap bisa dilihat detailnya -- yang hilang hanya
    // tindakan membukukannya.
    expect(await screen.findByRole('button', { name: /Detail/i })).toBeTruthy()
    await waitFor(() => expect(screen.queryByRole('button', { name: /Post ke GL/i })).toBeNull())
  })
})
