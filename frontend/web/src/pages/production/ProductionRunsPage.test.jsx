import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import ProductionRunsPage from './ProductionRunsPage.jsx'
import { PermissionProvider } from '../../store/PermissionContext.jsx'
import apiClient from '../../services/apiClient.js'
import { setSession, clearSession } from '../../utils/auth.js'

vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

function permissionRow(actions) {
  return {
    menu_id: 'menu-production-runs',
    menu_name: 'Eksekusi Produksi',
    menu_path: '/production/runs',
    module_id: 'mod-production',
    module_name: 'Production',
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
    if (url.includes('/work-orders')) {
      return Promise.resolve({ data: [{ id: 'wo-1', wo_number: 'WO-202608-0001', status: 'IN_PROGRESS', quantity_planned: 100 }] })
    }
    if (url.includes('/machines')) {
      return Promise.resolve({ data: [{ id: 'mc-1', code: 'MC-1', name: 'Mesin Cetak', status: 'ACTIVE' }] })
    }
    if (url.includes('/shifts')) {
      return Promise.resolve({ data: [{ id: 'sh-1', code: 'SH-1', name: 'Pagi', start_time: '08:00', end_time: '16:00', planned_minutes: 420, is_active: true }] })
    }
    return Promise.resolve({ data: [] })
  })
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/production/runs']}>
      <PermissionProvider>
        <ProductionRunsPage />
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

describe('ProductionRunsPage — gating tombol', () => {
  it('menampilkan "Mulai Eksekusi" saat user punya hak create', async () => {
    mockApi(permissionRow({ can_create: true }))

    renderPage()

    expect(await screen.findByRole('button', { name: /Mulai Eksekusi/i })).toBeTruthy()
  })

  it('menyembunyikan "Mulai Eksekusi" saat hak create dicabut', async () => {
    mockApi(permissionRow({ can_create: false }))

    renderPage()

    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith(
        '/api/rbac/user-permissions',
        expect.objectContaining({ params: expect.objectContaining({ user_id: 'user-1' }) })
      )
    )
    await waitFor(() => expect(screen.queryByRole('button', { name: /Mulai Eksekusi/i })).toBeNull())
    expect(screen.getByText('Eksekusi Produksi')).toBeTruthy()
  })

  // Halaman ini hanya boleh menawarkan work order yang sudah IN_PROGRESS --
  // production run untuk WO yang belum dimulai ditolak backend, dan menyodorkan
  // pilihan yang pasti gagal adalah cara terburuk menyampaikan aturan itu.
  it('hanya memuat work order IN_PROGRESS ke dropdown', async () => {
    apiClient.get.mockImplementation((url) => {
      if (url.includes('/user-permissions')) return Promise.resolve({ data: [permissionRow({ can_create: true })] })
      if (url.includes('/work-orders')) {
        return Promise.resolve({
          data: [
            { id: 'wo-1', wo_number: 'WO-202608-0001', status: 'DRAFT', quantity_planned: 100 },
            { id: 'wo-2', wo_number: 'WO-202608-0002', status: 'COMPLETED', quantity_planned: 50 },
          ],
        })
      }
      return Promise.resolve({ data: [] })
    })

    renderPage()

    // Tidak ada satu pun WO yang berjalan: tombolnya ada (hak create dipenuhi)
    // tapi mati, dan alasannya dijelaskan.
    const button = await screen.findByRole('button', { name: /Mulai Eksekusi/i })
    await waitFor(() => expect(button.disabled).toBe(true))
    expect(screen.getByText(/Belum ada Work Order berstatus IN_PROGRESS/i)).toBeTruthy()
  })
})
