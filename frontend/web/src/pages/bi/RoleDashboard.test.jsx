import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import SalesDashboardPage from './SalesDashboardPage.jsx'
import HrDashboardPage from './HrDashboardPage.jsx'
import ManufacturingDashboardPage from './ManufacturingDashboardPage.jsx'
import apiClient from '../../services/apiClient.js'

vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

function mockApi({ summary, analytics = {}, failAnalytics = [] } = {}) {
  apiClient.get.mockImplementation((url, config) => {
    if (url.includes('/ai-bi/dashboards/summary')) {
      return Promise.resolve({ data: summary ?? { company_id: 'company-1', sections: [], errors: [] } })
    }
    if (url.includes('/dw/analytics/')) {
      const endpoint = url.split('/dw/analytics/')[1]
      if (failAnalytics.includes(endpoint)) return Promise.reject(new Error('dw down'))
      return Promise.resolve({ data: analytics[endpoint] ?? [] })
    }
    return Promise.resolve({ data: [], config })
  })
}

function renderPage(Page) {
  return render(
    <MemoryRouter>
      <Page />
    </MemoryRouter>
  )
}

function summaryParamsFor(url) {
  const call = apiClient.get.mock.calls.find(([u]) => u.includes(url))
  return call?.[1]?.params
}

beforeEach(() => {
  apiClient.get.mockReset()
})

afterEach(cleanup)

describe('Dashboard per peran', () => {
  // Inti Fase 9: tiap dashboard hanya meminta bagian yang dipakainya, supaya
  // halaman Sales tidak ikut menunggu (atau ikut gagal karena) asset-service.
  it('meminta hanya bagian ringkasan yang dipakai halamannya', async () => {
    mockApi({ summary: { sales: { total_orders: 3, total_revenue: 1000, by_status: {} }, finance: {}, errors: [] } })

    renderPage(SalesDashboardPage)

    await waitFor(() => expect(summaryParamsFor('/ai-bi/dashboards/summary')).toBeTruthy())
    expect(summaryParamsFor('/ai-bi/dashboards/summary').sections).toBe('sales,finance')

    cleanup()
    apiClient.get.mockReset()
    mockApi({ summary: { hr: { total_employees: 10, active_employees: 8 }, errors: [] } })
    renderPage(HrDashboardPage)
    await waitFor(() => expect(summaryParamsFor('/ai-bi/dashboards/summary')).toBeTruthy())
    expect(summaryParamsFor('/ai-bi/dashboards/summary').sections).toBe('hr')
  })

  it('menampilkan angka ringkasan sebagai kartu', async () => {
    mockApi({
      summary: {
        hr: { total_employees: 120, active_employees: 113 },
        errors: [],
      },
    })

    renderPage(HrDashboardPage)

    expect(await screen.findByText('113')).toBeTruthy()
    // Nonaktif dihitung di halaman ini (total - aktif), bukan dikirim server.
    expect(screen.getByText('7')).toBeTruthy()
  })

  // Toleransi kegagalan sebagian, dua arah: service yang mati disebut namanya,
  // dan dw-service yang mati hanya mengosongkan grafiknya sendiri.
  it('menyebut service yang tidak bisa dihubungi tanpa mengosongkan sisanya', async () => {
    mockApi({
      summary: {
        production: { total_work_orders: 42, by_status: { IN_PROGRESS: 5 } },
        qc: { pass_rate_pct: 91.5, total_inspections: 200, fail_count: 4, partial_count: 2 },
        asset: {},
        errors: [{ source: 'asset-service', message: 'connection refused' }],
      },
      failAnalytics: ['qc-monthly-summary'],
    })

    renderPage(ManufacturingDashboardPage)

    expect(await screen.findByText(/asset-service tidak bisa dihubungi/i)).toBeTruthy()
    // Kartu dari service yang hidup tetap terisi.
    expect(screen.getByText('42')).toBeTruthy()
    expect(screen.getByText('91.5%')).toBeTruthy()
    // Grafik yang sumbernya gagal memberi tahu dirinya sendiri.
    await waitFor(() => expect(screen.getByText(/Gagal memuat data. Pastikan dw-service aktif/i)).toBeTruthy())
    // ...dan hanya dia: dua grafik lain tetap digambar.
    expect(screen.getAllByText(/Gagal memuat data/i)).toHaveLength(1)
  })

  it('mengonversi angka desimal berbentuk string dari dw-service', async () => {
    mockApi({
      summary: { production: { by_status: {} }, qc: {}, asset: {}, errors: [] },
      analytics: {
        'production-monthly-summary': [
          { month: '2026-07-01', quantity_planned: '100.00', quantity_produced: '95.50' },
        ],
      },
    })

    renderPage(ManufacturingDashboardPage)

    // Chart menolak menggambar kalau nilainya masih string; kalau konversinya
    // hilang, yang muncul adalah "Belum ada data." untuk grafik yang datanya ada.
    await waitFor(() => expect(screen.queryAllByText('Belum ada data.').length).toBe(2))
  })
})
