import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor, fireEvent } from '@testing-library/react'

import DataLakePage from './DataLakePage.jsx'
import apiClient from '../../services/apiClient.js'

vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

let canCreate = true
vi.mock('../../store/PermissionContext.jsx', () => ({
  usePagePermission: () => ({ can: (a) => (a === 'create' ? canCreate : true) }),
}))

const reconcile = {
  consistent: false,
  facts: [
    { fact: 'finance_journal_lines', silver_rows: 36, clickhouse_rows: 36, difference: 0, status: 'MATCH' },
    { fact: 'iot_readings', silver_rows: 1, clickhouse_rows: 22, difference: 21, status: 'MISSING_FROM_LAKE' },
  ],
}

function mockApi({ gold = {}, rec = reconcile } = {}) {
  apiClient.get.mockImplementation((url) => {
    if (url.includes('finance-monthly'))
      return Promise.resolve({ data: gold.finance ?? [{ company_id: 'company-1', month: '2026-07-01', revenue: '5000000', expense: '2000000' }] })
    if (url.includes('sales-monthly'))
      return Promise.resolve({ data: gold.sales ?? [{ company_id: 'company-1', month: '2026-07-01', sales_value: '24000' }] })
    if (url.includes('inventory-monthly'))
      return Promise.resolve({ data: gold.inventory ?? [{ company_id: 'company-1', month: '2026-07-01', stock_in: '120.5', stock_out: '30.25' }] })
    if (url.includes('downtime-monthly'))
      return Promise.resolve({
        data: gold.downtime ?? [
          { company_id: 'company-1', month: '2026-07-01', reason_code: 'BREAKDOWN', minutes: 40 },
          { company_id: 'company-1', month: '2026-07-01', reason_code: 'SETUP', minutes: 30 },
        ],
      })
    if (url.includes('oee-monthly'))
      return Promise.resolve({
        data: gold.oee ?? [
          { company_id: 'company-1', month: '2026-07-01', availability_pct: 85.71, performance_pct: 88.89, quality_pct: 93.75, oee_pct: 71.43 },
          { company_id: 'company-1', month: '2026-08-01', availability_pct: 90, performance_pct: null, quality_pct: null, oee_pct: null },
        ],
      })
    if (url.includes('qc-monthly'))
      return Promise.resolve({
        data: gold.qc ?? [{ company_id: 'company-1', month: '2026-07-01', pass_count: 4, partial_count: 1, fail_count: 2, defect_rate_pct: 3.5 }],
      })
    if (url.includes('production-monthly'))
      return Promise.resolve({
        data: gold.production ?? [{ company_id: 'company-1', month: '2026-07-01', quantity_planned: '170', quantity_produced: '80' }],
      })
    if (url.includes('ecommerce-monthly'))
      return Promise.resolve({
        data: gold.ecommerce ?? [{ company_id: 'company-1', month: '2026-07-01', order_count: 2, line_count: 3, items_sold: '5', revenue: '150500', avg_order_value: 75250 }],
      })
    if (url.includes('reconcile')) return Promise.resolve({ data: rec })
    return Promise.resolve({ data: [] })
  })
}

beforeEach(() => {
  apiClient.get.mockReset()
  apiClient.post.mockReset()
  canCreate = true
})

afterEach(cleanup)

describe('DataLakePage', () => {
  it('menampilkan tabel rekonsiliasi dengan status per fact', async () => {
    mockApi()
    render(<DataLakePage />)
    expect(await screen.findByText('Ada selisih')).toBeTruthy()
    expect(screen.getByText('Cocok')).toBeTruthy()
    expect(screen.getByText('Kurang di lake')).toBeTruthy()
    expect(screen.getByText('iot_readings')).toBeTruthy()
  })

  it('meminta Gold untuk company aktif', async () => {
    mockApi()
    render(<DataLakePage />)
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith('/api/dw/lake/gold/finance-monthly', { params: { company_id: 'company-1' } })
    )
  })

  it('menjelaskan dataset kosong alih-alih menggambar chart kosong', async () => {
    mockApi({ gold: { finance: [], sales: [], inventory: [] } })
    render(<DataLakePage />)
    expect((await screen.findAllByText(/Dataset Gold kosong/)).length).toBe(3)
  })

  it('meminta Gold OEE dan menggambar chartnya (bulan dengan faktor null tidak membuat halaman gagal)', async () => {
    mockApi()
    render(<DataLakePage />)
    expect(await screen.findByText('OEE per Bulan')).toBeTruthy()
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith('/api/dw/lake/gold/oee-monthly', { params: { company_id: 'company-1' } })
    )
    expect(screen.queryAllByText(/Dataset Gold kosong/).length).toBe(0)
  })

  it('meminta Gold QC dan menggambar chartnya', async () => {
    mockApi()
    render(<DataLakePage />)
    expect(await screen.findByText('Inspeksi QC per Bulan')).toBeTruthy()
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith('/api/dw/lake/gold/qc-monthly', { params: { company_id: 'company-1' } })
    )
    expect(screen.queryAllByText(/Dataset Gold kosong/).length).toBe(0)
  })

  it('meminta Gold produksi dan menggambar chartnya', async () => {
    mockApi()
    render(<DataLakePage />)
    expect(await screen.findByText('Rencana vs Realisasi Produksi per Bulan')).toBeTruthy()
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith('/api/dw/lake/gold/production-monthly', { params: { company_id: 'company-1' } })
    )
    expect(screen.queryAllByText(/Dataset Gold kosong/).length).toBe(0)
  })

  it('meminta Gold e-commerce dan menggambar chartnya', async () => {
    mockApi()
    render(<DataLakePage />)
    expect(await screen.findByText('Penjualan E-commerce per Bulan')).toBeTruthy()
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith('/api/dw/lake/gold/ecommerce-monthly', { params: { company_id: 'company-1' } })
    )
    expect(screen.queryAllByText(/Dataset Gold kosong/).length).toBe(0)
  })

  it('meminta Gold inventory dan menggambar chart stok', async () => {
    mockApi()
    render(<DataLakePage />)
    expect(await screen.findByText('Stok Masuk vs Keluar per Bulan')).toBeTruthy()
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith('/api/dw/lake/gold/inventory-monthly', { params: { company_id: 'company-1' } })
    )
    expect(screen.queryAllByText(/Dataset Gold kosong/).length).toBe(0)
  })

  it('menampilkan error build per fact dan memuat ulang Gold', async () => {
    mockApi()
    apiClient.post.mockRejectedValue({ response: { status: 207, data: { errors: ['silver iot_readings: boom'] } } })
    render(<DataLakePage />)
    fireEvent.click(await screen.findByText('Build Silver + Gold'))
    expect(await screen.findByText('silver iot_readings: boom')).toBeTruthy()
    const goldCalls = apiClient.get.mock.calls.filter(([u]) => u.includes('finance-monthly'))
    expect(goldCalls.length).toBe(2)
  })

  it('menyembunyikan tombol build tanpa izin create', async () => {
    canCreate = false
    mockApi()
    render(<DataLakePage />)
    await screen.findByText('Ada selisih')
    expect(screen.queryByText('Build Silver + Gold')).toBeNull()
  })
})
