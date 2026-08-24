import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import PredictiveMaintenancePage from './PredictiveMaintenancePage.jsx'
import ReorderRecommendationsPage from './ReorderRecommendationsPage.jsx'
import apiClient from '../../services/apiClient.js'

vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

function renderPage(Page) {
  return render(
    <MemoryRouter>
      <Page />
    </MemoryRouter>
  )
}

beforeEach(() => {
  apiClient.get.mockReset()
})

afterEach(cleanup)

describe('PredictiveMaintenancePage', () => {
  // Skor tanpa alasan akan diabaikan setelah tebakan pertamanya meleset, jadi
  // faktornya wajib ikut tergambar -- bukan hanya angkanya.
  it('menampilkan skor beserta faktor pembentuknya', async () => {
    apiClient.get.mockResolvedValue({
      data: {
        items: [
          {
            subject_type: 'machine',
            subject_id: 'mc-1',
            code: 'MC-001',
            name: 'Mesin Cetak',
            risk_score: 55,
            risk_level: 'MEDIUM',
            factors: [
              { code: 'LOW_AVAILABILITY', label: 'Availability di bawah 85%', contribution: 45, detail: '60% availability' },
              { code: 'QUALITY_DRAG', label: 'Tingkat reject tinggi', contribution: 10, detail: '90% unit lolos' },
            ],
            recommended_action: 'Periksa penyebab downtime terbesar mesin ini',
          },
        ],
        errors: [],
      },
    })

    renderPage(PredictiveMaintenancePage)

    expect(await screen.findByText('MC-001')).toBeTruthy()
    expect(screen.getByText('55')).toBeTruthy()
    expect(screen.getByText('MEDIUM')).toBeTruthy()
    expect(screen.getByText(/Availability di bawah 85%/)).toBeTruthy()
    expect(screen.getByText('+45')).toBeTruthy()
    expect(screen.getByText(/Periksa penyebab downtime/)).toBeTruthy()
    // Mesin dan aset dibedakan supaya orang tahu ke halaman mana harus pergi.
    expect(screen.getByText('Mesin')).toBeTruthy()
  })

  it('menyebut sumber yang gagal dihubungi', async () => {
    apiClient.get.mockResolvedValue({
      data: { items: [], errors: [{ source: 'production-service', message: 'connection refused' }] },
    })

    renderPage(PredictiveMaintenancePage)

    expect(await screen.findByText(/production-service/)).toBeTruthy()
    expect(screen.getByText(/belum lengkap/i)).toBeTruthy()
  })
})

describe('ReorderRecommendationsPage', () => {
  it('menampilkan dasar perhitungan dan sarannya', async () => {
    apiClient.get.mockResolvedValue({
      data: {
        window: { from: '2026-08-01', to: '2026-08-10', days: 10, movements_considered: 42, truncated: false },
        items: [
          {
            product_id: 'p-a',
            product_sku: 'SKU-A',
            product_name: 'Kertas A4',
            product_unit: 'rim',
            on_hand: 50,
            out_quantity: 100,
            daily_velocity: 10,
            days_of_cover: 5,
            suggested_reorder_qty: 250,
            urgency: 'HIGH',
            reason: 'Keluar 100 dalam 10 hari; sisa stok cukup untuk 5 hari',
          },
        ],
        errors: [],
      },
    })

    renderPage(ReorderRecommendationsPage)

    expect(await screen.findByText('SKU-A')).toBeTruthy()
    expect(screen.getByText('250')).toBeTruthy()
    expect(screen.getByText('HIGH')).toBeTruthy()
    expect(screen.getByText(/2026-08-01 s\/d 2026-08-10, 10 hari/)).toBeTruthy()
    expect(screen.queryByText(/terpotong di 200 baris/i)).toBeNull()
  })

  // Riwayat yang terpotong mengubah arti setiap angka di tabel; peringatannya
  // adalah bagian dari jawabannya, bukan hiasan.
  it('memperingatkan saat riwayat pergerakan terpotong', async () => {
    apiClient.get.mockResolvedValue({
      data: {
        window: { from: '2026-08-10', to: '2026-08-10', days: 1, movements_considered: 200, truncated: true },
        items: [],
        errors: [],
      },
    })

    renderPage(ReorderRecommendationsPage)

    expect(await screen.findByText(/terpotong di 200 baris terakhir/i)).toBeTruthy()
  })
})
