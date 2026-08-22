import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import InvoicesPage from './InvoicesPage.jsx'
import { PermissionProvider } from '../../store/PermissionContext.jsx'
import apiClient from '../../services/apiClient.js'
import { setSession, clearSession } from '../../utils/auth.js'

// Regresi: tombol Post di halaman ini dirender oleh `invoiceColumns()`, sebuah
// factory DI LUAR komponen. Saat gating dipasang, `can(...)` sempat dipakai di
// sana tanpa ada di scope-nya -- build tetap hijau (ini ReferenceError saat
// render, bukan kesalahan kompilasi) dan baru ketahuan oleh ESLint. Test ini
// menjaga supaya kolomnya benar-benar bisa dirender, bukan hanya bisa dibuild.
vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

const draftInvoice = {
  id: 'inv-1',
  invoice_number: 'INV-001',
  invoice_type: 'AR',
  partner_name: 'PT Contoh',
  invoice_date: '2026-08-01',
  due_date: '2026-08-31',
  total_amount: 1000000,
  status: 'DRAFT',
}

function mockApi({ canApprove }) {
  apiClient.get.mockImplementation((url) => {
    if (url.includes('/user-permissions')) {
      return Promise.resolve({
        data: [
          {
            menu_id: 'menu-invoices',
            menu_name: 'Invoice',
            menu_path: '/finance/invoices',
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
    if (url.includes('/invoices')) return Promise.resolve({ data: [draftInvoice] })
    return Promise.resolve({ data: [] })
  })
}

function renderInvoicesPage() {
  return render(
    <MemoryRouter initialEntries={['/finance/invoices']}>
      <PermissionProvider>
        <InvoicesPage />
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

describe('InvoicesPage — kolom aksi', () => {
  it('merender tombol Post untuk invoice DRAFT saat user boleh approve', async () => {
    mockApi({ canApprove: true })

    renderInvoicesPage()

    expect(await screen.findByText('INV-001')).toBeTruthy()
    expect(await screen.findByRole('button', { name: /^Post$/i })).toBeTruthy()
  })

  it('menyembunyikan tombol Post saat hak approve dicabut', async () => {
    mockApi({ canApprove: false })

    renderInvoicesPage()

    expect(await screen.findByText('INV-001')).toBeTruthy()
    await waitFor(() => expect(screen.queryByRole('button', { name: /^Post$/i })).toBeNull())
  })
})
