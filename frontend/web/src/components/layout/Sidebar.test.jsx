import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import Sidebar from './Sidebar.jsx'

// Sidebar kosong punya tiga sebab yang sangat berbeda dan tidak boleh terlihat
// sama: menu belum selesai dimuat, gagal dimuat, atau user memang tidak punya
// akses apa pun di company yang sedang dipilih. Sebab ketiga jadi jauh lebih
// sering sejak hak role di-scope per company, dan diam saja di situ membuatnya
// terlihat seperti aplikasi yang rusak.
function renderSidebar(props) {
  return render(
    <MemoryRouter initialEntries={['/dashboard']}>
      <Sidebar collapsed={false} onNavigate={() => {}} moduleTree={null} menuError="" {...props} />
    </MemoryRouter>
  )
}

afterEach(cleanup)

describe('Sidebar — keadaan kosong', () => {
  it('mengatakan "sedang memuat" selama menu belum datang', () => {
    renderSidebar({ moduleTree: null })

    expect(screen.getByText(/Memuat menu/i)).toBeTruthy()
    expect(screen.queryByText(/tidak punya akses/i)).toBeNull()
  })

  it('mengatakan tidak punya akses saat menu datang tapi isinya kosong', () => {
    renderSidebar({ moduleTree: [] })

    expect(screen.getByText(/tidak punya akses apa pun di company ini/i)).toBeTruthy()
    expect(screen.queryByText(/Memuat menu/i)).toBeNull()
  })

  it('menampilkan pesan gagal, bukan pesan tidak punya akses, saat menu gagal dimuat', () => {
    renderSidebar({ moduleTree: [], menuError: 'Gagal memuat menu dari server.' })

    expect(screen.getByText('Gagal memuat menu dari server.')).toBeTruthy()
    expect(screen.queryByText(/tidak punya akses/i)).toBeNull()
  })

  it('menampilkan modul apa adanya saat user punya akses', () => {
    renderSidebar({
      moduleTree: [{ id: 'mod-hr', name: 'HR', menus: [{ id: 'menu-leave', name: 'Cuti', path: '/hr/leave' }] }],
    })

    expect(screen.getByText('HR')).toBeTruthy()
    expect(screen.queryByText(/tidak punya akses/i)).toBeNull()
  })
})
