import { useEffect, useRef, useState } from 'react'

function RolePickerDropdown({ roles, disabled, onAssign, assigning }) {
  const [open, setOpen] = useState(false)
  const [checked, setChecked] = useState([])
  const [search, setSearch] = useState('')
  const rootRef = useRef(null)

  useEffect(() => {
    function onClickOutside(e) {
      if (rootRef.current && !rootRef.current.contains(e.target)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', onClickOutside)
    return () => document.removeEventListener('mousedown', onClickOutside)
  }, [])

  function toggleOpen() {
    if (disabled) return
    setOpen((v) => !v)
    setSearch('')
  }

  function toggleChecked(id) {
    setChecked((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  }

  async function handleApply() {
    await onAssign(checked)
    setChecked([])
    setOpen(false)
  }

  const filtered = roles.filter((r) => r.name.toLowerCase().includes(search.toLowerCase()))

  return (
    <div className="edp-role-picker" ref={rootRef}>
      <button type="button" className="btn btn-sm btn-outline-primary" onClick={toggleOpen} disabled={disabled}>
        <i className="bi bi-plus-lg me-1" />
        Tambah Role
      </button>

      {open && (
        <div className="edp-role-picker-panel card">
          <div className="p-2 border-bottom">
            <input
              type="text"
              className="form-control form-control-sm"
              placeholder="Cari role..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              autoFocus
            />
          </div>
          <div className="edp-role-picker-list">
            {filtered.length === 0 && <div className="text-secondary small text-center py-3">Tidak ada role.</div>}
            {filtered.map((r) => (
              <label key={r.id} className="edp-role-picker-item">
                <input
                  type="checkbox"
                  className="form-check-input"
                  checked={checked.includes(r.id)}
                  onChange={() => toggleChecked(r.id)}
                />
                <span className="flex-grow-1">{r.name}</span>
                <code className="small text-secondary">{r.code}</code>
              </label>
            ))}
          </div>
          <div className="p-2 border-top d-flex justify-content-between align-items-center">
            <span className="small text-secondary">{checked.length} dipilih</span>
            <button
              type="button"
              className="btn btn-sm btn-primary"
              disabled={checked.length === 0 || assigning}
              onClick={handleApply}
            >
              {assigning ? 'Menambahkan...' : 'Terapkan'}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export default RolePickerDropdown
