import { describe, it, expect } from 'vitest'
import { matchesPath, moduleContainsPath, findModuleIdForPath } from './menuTree.js'

// Bentuk data di bawah sengaja menyalin keluaran asli GET /api/rbac/menu-tree
// (module -> menus -> children), bukan bentuk yang dipermudah untuk test.
const moduleTree = [
  {
    id: 'mod-core',
    name: 'Core / Administrasi',
    menus: [
      { id: 'm-roles', name: 'Role Management', path: '/admin/roles', children: [] },
      { id: 'm-users', name: 'User Management', path: '/admin/users', children: [] },
    ],
  },
  {
    id: 'mod-hr',
    name: 'HR',
    menus: [
      {
        id: 'm-employees',
        name: 'Karyawan',
        path: '/hr/employees',
        children: [{ id: 'm-detail', name: 'Detail', path: '/hr/employees/detail', children: [] }],
      },
      { id: 'm-leave', name: 'Cuti', path: '/hr/leave', children: [] },
    ],
  },
]

describe('matchesPath', () => {
  it('cocok untuk path yang sama persis', () => {
    expect(matchesPath('/hr/leave', '/hr/leave')).toBe(true)
  })

  it('cocok untuk rute turunan yang tidak punya menu sendiri', () => {
    expect(matchesPath('/admin/roles', '/admin/roles/new')).toBe(true)
    expect(matchesPath('/admin/roles', '/admin/roles/abc-123/permissions')).toBe(true)
  })

  // Inti dari pencocokan berbatas segmen: tanpa itu "/admin/roles" akan ikut
  // mengklaim "/admin/roles-lain".
  it('tidak cocok kalau hanya sama di tengah segmen', () => {
    expect(matchesPath('/admin/roles', '/admin/roles-lain')).toBe(false)
    expect(matchesPath('/hr/leave', '/hr/leaves')).toBe(false)
  })

  it('tidak cocok untuk path kosong atau tidak ada', () => {
    expect(matchesPath('', '/hr/leave')).toBe(false)
    expect(matchesPath(undefined, '/hr/leave')).toBe(false)
    expect(matchesPath(null, '/hr/leave')).toBe(false)
  })
})

describe('moduleContainsPath', () => {
  it('menemukan menu level teratas', () => {
    expect(moduleContainsPath(moduleTree[1], '/hr/leave')).toBe(true)
  })

  it('menemukan menu anak', () => {
    expect(moduleContainsPath(moduleTree[1], '/hr/employees/detail')).toBe(true)
  })

  it('false untuk path milik modul lain', () => {
    expect(moduleContainsPath(moduleTree[0], '/hr/leave')).toBe(false)
  })

  it('aman untuk module tanpa menus', () => {
    expect(moduleContainsPath({ id: 'kosong' }, '/hr/leave')).toBe(false)
  })
})

describe('findModuleIdForPath', () => {
  it('mengembalikan id modul yang memuat halaman aktif', () => {
    expect(findModuleIdForPath(moduleTree, '/admin/users')).toBe('mod-core')
    expect(findModuleIdForPath(moduleTree, '/hr/leave')).toBe('mod-hr')
  })

  it('ikut mengenali rute turunan', () => {
    expect(findModuleIdForPath(moduleTree, '/admin/roles/new')).toBe('mod-core')
  })

  // Dashboard memang di luar accordion sidebar -- null, bukan modul pertama.
  it('null untuk halaman yang tidak ada menunya', () => {
    expect(findModuleIdForPath(moduleTree, '/')).toBeNull()
    expect(findModuleIdForPath(moduleTree, '/profile')).toBeNull()
  })

  it('null kalau menu belum dimuat', () => {
    expect(findModuleIdForPath(null, '/hr/leave')).toBeNull()
    expect(findModuleIdForPath(undefined, '/hr/leave')).toBeNull()
  })
})
