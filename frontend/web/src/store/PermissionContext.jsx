import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import { useLocation } from 'react-router-dom'
import apiClient from '../services/apiClient.js'
import { getCurrentUser } from '../utils/auth.js'
import { useCompany } from './CompanyContext.jsx'
import { matchesPath } from '../utils/menuTree.js'

// PermissionContext memuat hak EFEKTIF user yang sedang login (gabungan seluruh
// role-nya, lalu ditimpa override per user) sekali per company, lalu memakainya
// untuk menyembunyikan tombol aksi yang memang tidak boleh dia lakukan.
//
// PENTING: ini lapisan TAMPILAN. Penegakan sesungguhnya ada di api-gateway
// (backend/services/api-gateway/internal/authz), yang memeriksa hak SETIAP
// request sebelum meneruskannya ke service tujuan. Yang dikerjakan di sini
// hanya menjaga UI jujur: tidak menawarkan tombol yang ujungnya pasti ditolak.
// Karena itu aksi yang diminta tiap tombol harus SAMA dengan yang diminta
// tabel kebijakan di gateway -- mis. tombol "Post ke GL" memakai
// can('approve'), persis seperti POST /api/hr/payroll-runs/{id}/post di sana.
//
// Saat hak belum selesai dimuat atau gagal dimuat, jawabannya adalah BOLEH.
// Kebalikannya (default menolak) membuat aplikasi tampak rusak setiap kali
// rbac-service lambat -- dan tidak ada yang bocor karenanya, sebab request
// yang benar-benar dikirim tetap diperiksa gateway.

const PermissionContext = createContext(null)

const ACTION_KEYS = {
  view: 'can_view',
  create: 'can_create',
  update: 'can_update',
  delete: 'can_delete',
  approve: 'can_approve',
  export: 'can_export',
}

const ALL_ALLOWED = Object.freeze({
  can_view: true,
  can_create: true,
  can_update: true,
  can_delete: true,
  can_approve: true,
  can_export: true,
})

export function PermissionProvider({ children }) {
  const { companyId } = useCompany()
  const user = getCurrentUser()
  const isSuperAdmin = Boolean(user?.is_super_admin)
  const [byPath, setByPath] = useState(null)

  useEffect(() => {
    // Super admin melewati role & override sepenuhnya di backend (lihat
    // menuTree), jadi tidak ada gunanya menanyakan haknya per menu.
    if (!user || isSuperAdmin || !companyId) {
      setByPath(null)
      return
    }
    let cancelled = false
    apiClient
      .get('/api/rbac/user-permissions', { params: { user_id: user.id, company_id: companyId } })
      .then(({ data }) => {
        if (cancelled) return
        const map = {}
        data.forEach((row) => {
          if (row.menu_path) map[row.menu_path] = row
        })
        setByPath(map)
      })
      .catch(() => {
        if (!cancelled) setByPath(null)
      })
    return () => {
      cancelled = true
    }
    // getCurrentUser() mem-parse ulang JSON dari localStorage tiap render, jadi
    // objek `user` SELALU baru; menaruhnya di dependency array (seperti saran
    // exhaustive-deps) membuat efek ini berjalan tanpa henti. Yang benar-benar
    // menentukan pemuatan ulang hanyalah id-nya.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user?.id, isSuperAdmin, companyId])

  const value = useMemo(
    () => ({
      loaded: byPath !== null,
      // actionsFor mencari menu dengan path TERPANJANG yang mencakup halaman
      // ini, supaya rute turunan tanpa menu sendiri (mis. /admin/roles/new)
      // mewarisi hak menu induknya (/admin/roles).
      actionsFor(pathname) {
        if (isSuperAdmin || byPath === null) return ALL_ALLOWED
        let best = null
        Object.entries(byPath).forEach(([menuPath, row]) => {
          if (!matchesPath(menuPath, pathname)) return
          if (!best || menuPath.length > best.menuPath.length) best = { menuPath, row }
        })
        // Halaman tanpa menu sama sekali (mis. /profile) bukan urusan RBAC
        // menu -- jangan sembunyikan apa pun di sana.
        return best ? best.row : ALL_ALLOWED
      },
    }),
    [byPath, isSuperAdmin]
  )

  return <PermissionContext.Provider value={value}>{children}</PermissionContext.Provider>
}

export function usePermissions() {
  const ctx = useContext(PermissionContext)
  if (!ctx) throw new Error('usePermissions must be used within PermissionProvider')
  return ctx
}

// usePagePermission memberi helper `can` untuk halaman yang sedang dibuka:
//   const { can } = usePagePermission()
//   {can('create') && <button>Tambah</button>}
export function usePagePermission() {
  const { actionsFor, loaded } = usePermissions()
  const { pathname } = useLocation()
  const actions = actionsFor(pathname)
  return {
    loaded,
    actions,
    can(action) {
      const key = ACTION_KEYS[action]
      if (!key) throw new Error(`aksi tidak dikenal: ${action}`)
      return Boolean(actions[key])
    },
  }
}
