import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import CalibrationPage from './CalibrationPage.jsx'
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
    menu_id: 'menu-calibration',
    menu_name: 'Kalibrasi',
    menu_path: '/asset/calibration',
    module_id: 'mod-asset',
    module_name: 'Asset',
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

const asset = { id: 'asset-1', asset_code: 'AST-001', name: 'Timbangan Lab', status: 'ACTIVE' }

function mockApi(permissions, calibrations = []) {
  apiClient.get.mockImplementation((url) => {
    if (url.includes('/user-permissions')) return Promise.resolve({ data: [permissions] })
    if (url.includes('/calibrations')) return Promise.resolve({ data: calibrations })
    if (url.includes('/assets')) return Promise.resolve({ data: [asset] })
    return Promise.resolve({ data: [] })
  })
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/asset/calibration']}>
      <PermissionProvider>
        <CalibrationPage />
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

describe('CalibrationPage', () => {
  it('menampilkan "Jadwalkan Kalibrasi" saat user punya hak create', async () => {
    mockApi(permissionRow({ can_create: true }))

    renderPage()

    expect(await screen.findByRole('button', { name: /Jadwalkan Kalibrasi/i })).toBeTruthy()
  })

  it('menyembunyikan "Jadwalkan Kalibrasi" saat hak create dicabut', async () => {
    mockApi(permissionRow({ can_create: false }))

    renderPage()

    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith(
        '/api/rbac/user-permissions',
        expect.objectContaining({ params: expect.objectContaining({ user_id: 'user-1' }) })
      )
    )
    await waitFor(() => expect(screen.queryByRole('button', { name: /Jadwalkan Kalibrasi/i })).toBeNull())
    expect(screen.getByText('Kalibrasi')).toBeTruthy()
  })

  // Overdue tidak disimpan sebagai status di database; halaman ini yang
  // menghitungnya saat menggambar, jadi di sinilah aturannya perlu dijaga.
  it('menandai jadwal yang sudah lewat sebagai Terlambat, dan yang belum tidak', async () => {
    mockApi(permissionRow({ can_view: true }), [
      { id: 'cal-late', asset_id: 'asset-1', scheduled_date: '2020-01-05', status: 'SCHEDULED', interval_months: 12, notes: '' },
      { id: 'cal-future', asset_id: 'asset-1', scheduled_date: '2099-01-05', status: 'SCHEDULED', interval_months: 12, notes: '' },
      { id: 'cal-done', asset_id: 'asset-1', scheduled_date: '2020-02-05', performed_date: '2020-02-06', result: 'PASS', status: 'COMPLETED', notes: '' },
    ])

    renderPage()

    await screen.findByText('5/1/2020')
    // Hanya satu baris yang terlambat: yang COMPLETED tidak ikut walau
    // tanggalnya juga sudah lewat.
    expect(screen.getAllByText('Terlambat')).toHaveLength(1)
    expect(screen.getByText('Lolos')).toBeTruthy()
  })
})
