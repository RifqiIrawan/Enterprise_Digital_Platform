import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react'

import BomPage from './BomPage.jsx'
import apiClient from '../../services/apiClient.js'

vi.mock('../../services/apiClient.js', () => ({
  default: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}))

vi.mock('../../store/CompanyContext.jsx', () => ({
  useCompany: () => ({ companyId: 'company-1', branchId: null }),
}))

vi.mock('../../store/PermissionContext.jsx', () => ({
  usePagePermission: () => ({ can: () => true }),
}))

const products = [
  { id: 'p-1', sku: 'RM-1', name: 'Tepung' },
  { id: 'p-2', sku: 'RM-2', name: 'Gula' },
]

const formula = {
  id: 'bom-1',
  bom_code: 'FRM-1',
  name: 'Roti Manis',
  product_id: 'p-9',
  is_active: true,
  bom_type: 'BATCH',
  batch_size: 1000,
}

function mockApi(detailLines) {
  apiClient.get.mockImplementation((url) => {
    if (url === '/api/production/boms') return Promise.resolve({ data: [formula] })
    if (url === '/api/production/boms/bom-1') return Promise.resolve({ data: { ...formula, lines: detailLines } })
    if (url.includes('/products')) return Promise.resolve({ data: products })
    return Promise.resolve({ data: [] })
  })
  apiClient.put.mockResolvedValue({ data: {} })
}

beforeEach(() => {
  apiClient.get.mockReset()
  apiClient.put.mockReset()
})

afterEach(cleanup)

describe('BomPage edit', () => {
  it('memuat komponen formula dari detail BOM dan mengirim percentage + batch_size saat disimpan', async () => {
    mockApi([
      { component_product_id: 'p-1', quantity_per_unit: 0.6, percentage: 60 },
      { component_product_id: 'p-2', quantity_per_unit: 0.4, percentage: 40 },
    ])
    render(<BomPage />)

    fireEvent.click(await screen.findByRole('button', { name: 'Edit BOM FRM-1' }))
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith('/api/production/boms/bom-1'))
    // Dua baris komponen termuat: total 100%.
    expect(await screen.findByText(/Total 100%/)).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Simpan' }))
    await waitFor(() => expect(apiClient.put).toHaveBeenCalled())
    const [url, body] = apiClient.put.mock.calls[0]
    expect(url).toBe('/api/production/boms/bom-1')
    expect(body).toEqual({
      name: 'Roti Manis',
      is_active: true,
      batch_size: 1000,
      lines: [
        { component_product_id: 'p-1', percentage: 60 },
        { component_product_id: 'p-2', percentage: 40 },
      ],
    })
  })

  it('menampilkan galat backend dan tidak menutup dialog', async () => {
    mockApi([{ component_product_id: 'p-1', quantity_per_unit: 1, percentage: 100 }])
    apiClient.put.mockRejectedValue({ response: { data: { error: 'Jumlah percentage kurang dari 100%' } } })
    render(<BomPage />)

    fireEvent.click(await screen.findByRole('button', { name: 'Edit BOM FRM-1' }))
    await screen.findByText(/Total 100%/)
    fireEvent.click(screen.getByRole('button', { name: 'Simpan' }))
    expect(await screen.findByText('Jumlah percentage kurang dari 100%')).toBeTruthy()
    expect(screen.getByText('Edit BOM FRM-1')).toBeTruthy()
  })
})
