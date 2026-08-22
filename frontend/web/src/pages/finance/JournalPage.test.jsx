import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import JournalPage from './JournalPage.jsx'
import { PermissionProvider } from '../../store/PermissionContext.jsx'
import apiClient from '../../services/apiClient.js'
import { setSession, clearSession } from '../../utils/auth.js'

// Pasangan dari InvoicesPage.test.jsx: tombol Post di halaman ini juga
// dirender oleh factory kolom di luar komponen (journalColumns), tempat
// `can(...)` sempat dipakai tanpa ada di scope-nya. Build tidak bisa melihat
// kesalahan seperti itu -- render-nya yang bisa.
vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

const draftEntry = {
  id: 'je-1',
  entry_number: 'JE-001',
  entry_date: '2026-08-01',
  description: 'Jurnal uji',
  reference_type: 'MANUAL',
  total_debit: 500000,
  total_credit: 500000,
  status: 'DRAFT',
}

function mockApi({ canApprove }) {
  apiClient.get.mockImplementation((url) => {
    if (url.includes('/user-permissions')) {
      return Promise.resolve({
        data: [
          {
            menu_id: 'menu-journal',
            menu_name: 'Jurnal',
            menu_path: '/finance/journal',
            module_id: 'mod-finance',
            module_name: 'Finance',
            can_view: true,
            can_create: false,
            can_update: false,
            can_delete: false,
            can_approve: canApprove,
            can_export: false,
            source: 'override',
            role_actions: {},
          },
        ],
      })
    }
    if (url.includes('journal')) return Promise.resolve({ data: [draftEntry] })
    return Promise.resolve({ data: [] })
  })
}

function renderJournalPage() {
  return render(
    <MemoryRouter initialEntries={['/finance/journal']}>
      <PermissionProvider>
        <JournalPage />
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

describe('JournalPage — kolom aksi', () => {
  it('merender tombol Post untuk jurnal DRAFT saat user boleh approve', async () => {
    mockApi({ canApprove: true })

    renderJournalPage()

    expect(await screen.findByText('JE-001')).toBeTruthy()
    expect(await screen.findByRole('button', { name: /^Post$/i })).toBeTruthy()
  })

  it('menyembunyikan tombol Post saat hak approve dicabut', async () => {
    mockApi({ canApprove: false })

    renderJournalPage()

    expect(await screen.findByText('JE-001')).toBeTruthy()
    await waitFor(() => expect(screen.queryByRole('button', { name: /^Post$/i })).toBeNull())
  })
})
