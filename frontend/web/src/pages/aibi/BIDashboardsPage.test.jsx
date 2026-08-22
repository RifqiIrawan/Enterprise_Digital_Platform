import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import BIDashboardsPage from './BIDashboardsPage.jsx'
import { PermissionProvider } from '../../store/PermissionContext.jsx'
import apiClient from '../../services/apiClient.js'
import { setSession, clearSession } from '../../utils/auth.js'

// Halaman ini dipecah jadi 4 tab bertema karena sudah memuat 9 grafik. Yang
// dijaga test ini adalah pembagiannya benar-benar bekerja: grafik SDM tidak
// ikut terender saat tab Keuangan aktif, dan sebaliknya. Tanpa ini,
// pembagiannya hanya terbukti lewat build yang hijau -- dan build tidak tahu
// grafik mana yang seharusnya ada di tab mana.
vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

const summary = {
  generated_at: '2026-08-21T10:00:00Z',
  errors: [],
  sales: { total_orders: 12, total_revenue: 1000, by_status: { DRAFT: 2 } },
  purchasing: { total_orders: 3, total_spend: 500, by_status: { DRAFT: 1 } },
  finance: { ar_outstanding: 10, ar_total: 20, ap_outstanding: 5, ap_total: 10, journal_entries_count: 7 },
  warehouse: { total_products: 4, total_warehouses: 2, total_stock_lines: 9, low_stock_count: 1 },
  hr: { active_employees: 6, total_employees: 8 },
  production: { total_work_orders: 5, by_status: { DRAFT: 5 } },
  qc: { pass_count: 3, fail_count: 1, partial_count: 0, pass_rate_pct: 75 },
  asset: { total_assets: 2, overdue_maintenance_count: 0 },
}

function mockApi() {
  apiClient.get.mockImplementation((url) => {
    if (url.includes('/dashboards/summary')) return Promise.resolve({ data: summary })
    // Seluruh endpoint analytics mengembalikan daftar kosong: yang diuji di
    // sini penempatan kartunya, bukan isi grafiknya.
    return Promise.resolve({ data: [] })
  })
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/ai-bi/dashboards']}>
      <PermissionProvider>
        <BIDashboardsPage />
      </PermissionProvider>
    </MemoryRouter>
  )
}

beforeEach(() => {
  clearSession()
  setSession('token-uji', { id: 'user-1', email: 'u@edp.test', full_name: 'U', is_super_admin: true })
  apiClient.get.mockReset()
  mockApi()
})

afterEach(() => {
  cleanup()
  clearSession()
})

describe('BIDashboardsPage — tab bertema', () => {
  it('membuka tab Ringkasan lebih dulu', async () => {
    renderPage()

    expect(await screen.findByText('Sales — Total Order')).toBeTruthy()
    // Grafik tab lain tidak ikut terender.
    expect(screen.queryByText(/Hari Cuti per Bulan/)).toBeNull()
    expect(screen.queryByText(/Revenue vs Expense per Bulan/)).toBeNull()
  })

  it('pindah ke tab SDM menampilkan tiga grafik HR dan menyembunyikan ringkasan', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Sales — Total Order')

    await user.click(screen.getByRole('button', { name: 'SDM' }))

    expect(await screen.findByText(/Hari Cuti per Bulan/)).toBeTruthy()
    expect(screen.getByText(/Sebaran Rating KPI/)).toBeTruthy()
    expect(screen.getByText(/Rata-rata KPI per Departemen/)).toBeTruthy()
    expect(screen.queryByText('Sales — Total Order')).toBeNull()
  })

  // Empat grafik baru (QC, produksi, belanja supplier, tiket) harus mendarat di
  // tab yang benar -- ini yang gampang salah saat menambah kartu ke halaman
  // yang sudah punya empat panel.
  it('grafik baru berada di tab temanya masing-masing', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Sales — Total Order')

    await user.click(screen.getByRole('button', { name: 'Operasional' }))
    expect(await screen.findByText(/Hasil Inspeksi QC per Bulan/)).toBeTruthy()
    expect(screen.getByText(/Rencana vs Realisasi Produksi/)).toBeTruthy()
    expect(screen.getByText(/Tiket per Bulan/)).toBeTruthy()
    expect(screen.queryByText(/Belanja per Supplier/)).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Keuangan & Penjualan' }))
    expect(await screen.findByText(/Belanja per Supplier/)).toBeTruthy()
    expect(screen.queryByText(/Hasil Inspeksi QC per Bulan/)).toBeNull()
  })

  // Empat kartu terakhir (payroll, perawatan aset, sensor, penjualan online)
  // melengkapi 16 fact table -- masing-masing harus mendarat di tab temanya.
  it('empat kartu terakhir berada di tab yang benar', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Sales — Total Order')

    await user.click(screen.getByRole('button', { name: 'SDM' }))
    expect(await screen.findByText(/Payroll per Periode/)).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Operasional' }))
    expect(await screen.findByText(/Perawatan Aset per Bulan/)).toBeTruthy()
    expect(screen.getByText(/Sensor IoT/)).toBeTruthy()
    expect(screen.queryByText(/Payroll per Periode/)).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Keuangan & Penjualan' }))
    expect(await screen.findByText(/Penjualan Online per Bulan/)).toBeTruthy()
    expect(screen.queryByText(/Sensor IoT/)).toBeNull()
  })

  it('tab Keuangan & Penjualan tidak memuat grafik operasional, dan sebaliknya', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Sales — Total Order')

    await user.click(screen.getByRole('button', { name: 'Keuangan & Penjualan' }))
    expect(await screen.findByText(/Revenue vs Expense per Bulan/)).toBeTruthy()
    expect(screen.getByText(/Pipeline CRM per Stage/)).toBeTruthy()
    expect(screen.queryByText(/Stok Masuk vs Keluar per Bulan/)).toBeNull()
    expect(screen.queryByText(/Pengiriman per Bulan/)).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Operasional' }))
    expect(await screen.findByText(/Stok Masuk vs Keluar per Bulan/)).toBeTruthy()
    expect(screen.getByText(/Pengiriman per Bulan/)).toBeTruthy()
    expect(screen.queryByText(/Revenue vs Expense per Bulan/)).toBeNull()
  })

  // Seluruh data tetap diambil sekali saat halaman dibuka, bukan per tab:
  // berpindah tab harus terasa seketika, dan datanya kecil (satu baris per
  // bulan/periode).
  it('memuat semua endpoint sekali di awal, tidak per tab', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Sales — Total Order')

    const callsAfterLoad = apiClient.get.mock.calls.length
    await user.click(screen.getByRole('button', { name: 'SDM' }))
    await screen.findByText(/Hari Cuti per Bulan/)

    expect(apiClient.get.mock.calls.length).toBe(callsAfterLoad)
    const urls = apiClient.get.mock.calls.map(([url]) => url)
    expect(urls).toContain('/api/dw/analytics/hr-kpi-department-summary')
    expect(urls).toContain('/api/dw/analytics/hr-leave-monthly-summary')
  })

  // Pemilih periode: tanpa pilihan, backend yang menentukan periode terakhir;
  // begitu dipilih, periodenya ikut dikirim sebagai parameter.
  it('pemilih periode mengirim ulang permintaan dengan period yang dipilih', async () => {
    apiClient.get.mockImplementation((url) => {
      if (url.includes('/dashboards/summary')) return Promise.resolve({ data: summary })
      if (url.includes('hr-kpi-summary')) {
        return Promise.resolve({
          data: [
            { period: '2026-07', review_count: 1, approved_count: 1, avg_score: 70, sangat_baik_count: 0, baik_count: 0, cukup_count: 1, perlu_perbaikan_count: 0 },
            { period: '2026-08', review_count: 1, approved_count: 1, avg_score: 90, sangat_baik_count: 1, baik_count: 0, cukup_count: 0, perlu_perbaikan_count: 0 },
          ],
        })
      }
      return Promise.resolve({ data: [] })
    })

    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Sales — Total Order')
    await user.click(screen.getByRole('button', { name: 'SDM' }))

    const select = await screen.findByLabelText('Periode KPI')
    // Permintaan pertama sengaja TANPA period.
    const firstDeptCall = apiClient.get.mock.calls.find(([url]) => url.includes('hr-kpi-department-summary'))
    expect(firstDeptCall[1].params.period).toBeUndefined()

    await user.selectOptions(select, '2026-07')

    const deptCalls = apiClient.get.mock.calls.filter(([url]) => url.includes('hr-kpi-department-summary'))
    expect(deptCalls.length).toBe(2)
    expect(deptCalls[1][1].params.period).toBe('2026-07')
  })
})
