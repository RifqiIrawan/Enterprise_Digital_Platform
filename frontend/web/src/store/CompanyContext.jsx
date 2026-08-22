import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import apiClient from '../services/apiClient.js'

const COMPANY_KEY = 'current_company_id'
const BRANCH_KEY = 'current_branch_id'

// CompanyContext dipasang sekali di MainLayout (lihat komentar di sana),
// menggantikan pola lama tiap halaman fetch `/api/company/companies` sendiri
// dan asal ambil data[0]. Company dipilih lewat CompanySwitcher di Topbar;
// branch cuma berfungsi sebagai default branch_id di form create (bukan
// filter list di backend -- belum ada endpoint yang mendukung itu, lihat
// NEXT_SESSION.md).
const CompanyContext = createContext(null)

export function CompanyProvider({ children }) {
  const [companies, setCompanies] = useState([])
  const [branches, setBranches] = useState([])
  const [companyId, setCompanyIdState] = useState('')
  const [branchId, setBranchIdState] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // reloadCompanies/reloadBranches dipakai halaman Company & Branch Management
  // setelah menambah/mengubah/menghapus data. Tanpa ini, switcher di Topbar
  // tetap menampilkan daftar hasil fetch pertama sampai halaman di-refresh
  // penuh -- termasuk company yang baru saja dibuat (tidak muncul) atau branch
  // yang baru saja dihapus (masih bisa dipilih).
  const reloadCompanies = useCallback(
    () =>
      apiClient
        .get('/api/company/companies')
        .then(({ data }) => {
          setCompanies(data)
          setCompanyIdState((current) => {
            if (current && data.some((c) => c.id === current)) return current
            const savedId = localStorage.getItem(COMPANY_KEY)
            return (savedId && data.some((c) => c.id === savedId) ? savedId : data[0]?.id) ?? ''
          })
        })
        .catch(() => setError('Gagal memuat data company.')),
    []
  )

  const reloadBranches = useCallback((id) => {
    if (!id) {
      setBranches([])
      setBranchIdState('')
      return Promise.resolve()
    }
    return apiClient
      .get(`/api/company/companies/${id}/branches`)
      .then(({ data }) => {
        setBranches(data)
        // Branch aktif yang sudah tidak ada lagi (dihapus) harus dilepas,
        // kalau tidak form create di halaman lain akan mengirim branch_id
        // yatim.
        setBranchIdState((current) => {
          if (current && data.some((b) => b.id === current)) return current
          const savedBranch = localStorage.getItem(BRANCH_KEY)
          if (data.some((b) => b.id === savedBranch)) return savedBranch
          localStorage.removeItem(BRANCH_KEY)
          return ''
        })
      })
      .catch(() => setBranches([]))
  }, [])

  useEffect(() => {
    reloadCompanies().finally(() => setLoading(false))
  }, [reloadCompanies])

  useEffect(() => {
    if (companyId) localStorage.setItem(COMPANY_KEY, companyId)
    reloadBranches(companyId)
  }, [companyId, reloadBranches])

  const setCompanyId = useCallback((id) => setCompanyIdState(id), [])

  const setBranchId = useCallback((id) => {
    setBranchIdState(id)
    if (id) localStorage.setItem(BRANCH_KEY, id)
    else localStorage.removeItem(BRANCH_KEY)
  }, [])

  const value = useMemo(
    () => ({
      companies,
      branches,
      companyId,
      branchId,
      setCompanyId,
      setBranchId,
      reloadCompanies,
      reloadBranches,
      loading,
      error,
    }),
    [companies, branches, companyId, branchId, setCompanyId, setBranchId, reloadCompanies, reloadBranches, loading, error]
  )

  return <CompanyContext.Provider value={value}>{children}</CompanyContext.Provider>
}

export function useCompany() {
  const ctx = useContext(CompanyContext)
  if (!ctx) throw new Error('useCompany must be used within CompanyProvider')
  return ctx
}
