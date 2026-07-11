import { useCompany } from '../../store/CompanyContext.jsx'

function CompanySwitcher() {
  const { companies, branches, companyId, branchId, setCompanyId, setBranchId, loading } = useCompany()

  if (loading || companies.length === 0) return null

  return (
    <div className="d-flex align-items-center gap-2">
      <select
        className="form-select form-select-sm"
        style={{ width: 180 }}
        value={companyId}
        onChange={(e) => setCompanyId(e.target.value)}
        aria-label="Pilih company"
      >
        {companies.map((c) => (
          <option key={c.id} value={c.id}>{c.name}</option>
        ))}
      </select>
      {branches.length > 0 && (
        <select
          className="form-select form-select-sm"
          style={{ width: 160 }}
          value={branchId}
          onChange={(e) => setBranchId(e.target.value)}
          aria-label="Pilih branch"
        >
          <option value="">Semua Branch</option>
          {branches.map((b) => (
            <option key={b.id} value={b.id}>{b.name}</option>
          ))}
        </select>
      )}
    </div>
  )
}

export default CompanySwitcher
