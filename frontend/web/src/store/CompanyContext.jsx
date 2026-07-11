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

  useEffect(() => {
    apiClient
      .get('/api/company/companies')
      .then(({ data }) => {
        setCompanies(data)
        const savedId = localStorage.getItem(COMPANY_KEY)
        const initial = (savedId && data.some((c) => c.id === savedId) ? savedId : data[0]?.id) ?? ''
        setCompanyIdState(initial)
      })
      .catch(() => setError('Gagal memuat data company.'))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (!companyId) {
      setBranches([])
      setBranchIdState('')
      return
    }
    localStorage.setItem(COMPANY_KEY, companyId)
    apiClient
      .get(`/api/company/companies/${companyId}/branches`)
      .then(({ data }) => {
        setBranches(data)
        const savedBranch = localStorage.getItem(BRANCH_KEY)
        setBranchIdState(data.some((b) => b.id === savedBranch) ? savedBranch : '')
      })
      .catch(() => setBranches([]))
  }, [companyId])

  const setCompanyId = useCallback((id) => setCompanyIdState(id), [])

  const setBranchId = useCallback((id) => {
    setBranchIdState(id)
    if (id) localStorage.setItem(BRANCH_KEY, id)
    else localStorage.removeItem(BRANCH_KEY)
  }, [])

  const value = useMemo(
    () => ({ companies, branches, companyId, branchId, setCompanyId, setBranchId, loading, error }),
    [companies, branches, companyId, branchId, setCompanyId, setBranchId, loading, error]
  )

  return <CompanyContext.Provider value={value}>{children}</CompanyContext.Provider>
}

export function useCompany() {
  const ctx = useContext(CompanyContext)
  if (!ctx) throw new Error('useCompany must be used within CompanyProvider')
  return ctx
}
