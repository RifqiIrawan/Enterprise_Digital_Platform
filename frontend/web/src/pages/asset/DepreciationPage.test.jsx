import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import DepreciationPage from './DepreciationPage.jsx'
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
    menu_id: 'menu-depreciation',
    menu_name: 'Penyusutan',
    menu_path: '/asset/depreciation',
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

const draftRun = { id: 'run-1', period: '2026-08', status: 'DRAFT', asset_count: 2, total_amount: 1500000, journal_entry_id: null, posted_at: null }

const draftDetail = {
  ...draftRun,
  entries: [
    { id: 'e1', asset_code: 'AST-001', asset_name: 'Mesin Cetak', method: 'STRAIGHT_LINE', amount: 1000000, book_value_before: 12000000, book_value_after: 11000000 },
    { id: 'e2', asset_code: 'AST-002', asset_name: 'Forklift', method: 'DECLINING_BALANCE', amount: 500000, book_value_before: 5000000, book_value_after: 4500000 },
  ],
}

function mockApi(permissions) {
  apiClient.get.mockImplementation((url) => {
    if (url.includes('/user-permissions')) return Promise.resolve({ data: [permissions] })
    if (url.includes('/depreciation-runs/')) return Promise.resolve({ data: draftDetail })
    if (url.includes('/depreciation-runs')) return Promise.resolve({ data: [draftRun] })
    if (url.includes('/finance/accounts')) {
      return Promise.resolve({
        data: [
          { id: 'acc-1', account_code: '6100', account_name: 'Beban Penyusutan' },
          { id: 'acc-2', account_code: '1290', account_name: 'Akumulasi Penyusutan' },
        ],
      })
    }
    return Promise.resolve({ data: [] })
  })
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/asset/depreciation']}>
      <PermissionProvider>
        <DepreciationPage />
      </PermissionProvider>
    </MemoryRouter>
  )
}

beforeEach(() => {
  clearSession()
  setSession('token-uji', { id: 'user-1', email: 'u@edp.test', full_name: 'U', is_super_admin: false })
  apiClient.get.mockReset()
  apiClient.post.mockReset()
})

afterEach(() => {
  cleanup()
  clearSession()
})

describe('DepreciationPage — gating', () => {
  it('menampilkan "Hitung Penyusutan" saat user punya hak create', async () => {
    mockApi(permissionRow({ can_create: true }))

    renderPage()

    expect(await screen.findByRole('button', { name: /Hitung Penyusutan/i })).toBeTruthy()
  })

  it('menyembunyikan "Hitung Penyusutan" saat hak create dicabut', async () => {
    mockApi(permissionRow({ can_create: false }))

    renderPage()

    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith(
        '/api/rbac/user-permissions',
        expect.objectContaining({ params: expect.objectContaining({ user_id: 'user-1' }) })
      )
    )
    await waitFor(() => expect(screen.queryByRole('button', { name: /Hitung Penyusutan/i })).toBeNull())
    expect(screen.getByText('Penyusutan')).toBeTruthy()
  })

  // Memposting ke buku besar adalah hak tersendiri (approve), terpisah dari
  // menghitung. Hak create saja tidak boleh memunculkan tombol postingnya.
  it('hanya menampilkan "Post ke GL" di rincian saat user punya hak approve', async () => {
    mockApi(permissionRow({ can_create: true, can_approve: false }))

    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: /Rincian/i }))

    // Rinciannya tetap terbaca -- yang dicabut hanya aksinya.
    expect(await screen.findByText('Mesin Cetak')).toBeTruthy()
    expect(screen.getByText('Forklift')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Post ke GL/i })).toBeNull()

    cleanup()
    mockApi(permissionRow({ can_create: true, can_approve: true }))
    renderPage()
    await userEvent.click(await screen.findByRole('button', { name: /Rincian/i }))
    expect(await screen.findByRole('button', { name: /Post ke GL/i })).toBeTruthy()
  })
})
