import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import OeePage from './OeePage.jsx'
import apiClient from '../../services/apiClient.js'

vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

const metrics = {
  planned_minutes: 420,
  downtime_minutes: 60,
  run_time_minutes: 360,
  quantity_good: 150,
  quantity_reject: 10,
  availability: 0.8571,
  performance: 0.8889,
  quality: 0.9375,
  oee: 0.7143,
  performance_capped: false,
}

function mockApi(summary) {
  apiClient.get.mockImplementation((url) => {
    if (url.includes('/oee')) return Promise.resolve({ data: summary })
    if (url.includes('/machines')) return Promise.resolve({ data: [{ id: 'mc-1', code: 'MC-1', name: 'Mesin Cetak' }] })
    return Promise.resolve({ data: [] })
  })
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/production/oee']}>
      <OeePage />
    </MemoryRouter>
  )
}

beforeEach(() => {
  apiClient.get.mockReset()
})

afterEach(cleanup)

describe('OeePage', () => {
  it('menampilkan tiga faktor dan OEE sebagai persen', async () => {
    mockApi({
      overall: metrics,
      machines: [{ machine_id: 'mc-1', machine_code: 'MC-1', machine_name: 'Mesin Cetak', run_count: 2, ...metrics }],
      downtime_by_reason: [
        { reason_code: 'BREAKDOWN', minutes: 45, share: 0.75 },
        { reason_code: 'SETUP', minutes: 15, share: 0.25 },
      ],
    })

    renderPage()

    // Pecahan 0..1 dari server, persen di layar -- pembulatannya milik halaman ini.
    expect(await screen.findAllByText('71.4%')).toHaveLength(2) // stat tile + baris tabel
    expect(screen.getAllByText('85.7%').length).toBeGreaterThan(0)
    expect(screen.getAllByText('88.9%').length).toBeGreaterThan(0)
    expect(screen.getAllByText('93.8%').length).toBeGreaterThan(0)

    // Kode alasan diterjemahkan, bukan ditampilkan mentah.
    expect(screen.getByText('Kerusakan mesin')).toBeTruthy()
    expect(screen.getByText(/45 menit · 75.0%/)).toBeTruthy()
  })

  it('memberi tahu saat Performance dipotong ke 100%', async () => {
    const capped = { ...metrics, performance: 1, performance_capped: true }
    mockApi({
      overall: capped,
      machines: [{ machine_id: 'mc-1', machine_code: 'MC-1', machine_name: 'Mesin Cetak', run_count: 1, ...capped }],
      downtime_by_reason: [],
    })

    renderPage()

    expect(await screen.findByText(/Performance dipotong ke 100%/i)).toBeTruthy()
  })

  it('tidak menampilkan 0% saat belum ada run yang ditutup', async () => {
    // Server mengirim angka nol untuk periode kosong; 0% bukan hal yang sama
    // dengan "belum ada yang bisa diukur".
    mockApi({
      overall: { ...metrics, planned_minutes: 0, downtime_minutes: 0, run_time_minutes: 0, quantity_good: 0, quantity_reject: 0, availability: 0, performance: 0, quality: 0, oee: 0 },
      machines: [],
      downtime_by_reason: [],
    })

    renderPage()

    expect(await screen.findByText(/belum ada run tertutup pada periode ini/i)).toBeTruthy()
    expect(screen.getByText(/Belum ada eksekusi produksi yang ditutup/i)).toBeTruthy()
    expect(screen.queryByText('0.0%')).toBeNull()
    expect(screen.getAllByText('—').length).toBe(4) // OEE, Availability, Performance, Quality
  })

  it('mengisi "Dari" dengan tanggal 1 bulan ini menurut zona waktu lokal', async () => {
    // 15 Sep 00:30 lokal: toISOString() di zona positif (mis. UTC+7) menghasilkan
    // "Dari" 31 Agustus dan "Sampai" 14 September. Di zona UTC test ini lolos
    // dengan kode lama juga, jadi ia hanya menjaga di mesin ber-zona positif.
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 8, 15, 0, 30))
    try {
      mockApi({ overall: metrics, machines: [], downtime_by_reason: [] })
      renderPage()
      await screen.findByText(/Rincian per Mesin/i)
      const [from, to] = document.querySelectorAll('input[type="date"]')
      expect(from.value).toBe('2026-09-01')
      expect(to.value).toBe('2026-09-15')
    } finally {
      vi.useRealTimers()
    }
  })
})
