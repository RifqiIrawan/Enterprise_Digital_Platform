import { useMemo, useState } from 'react'

function buildPageList(totalPages, currentPage) {
  const pages = Array.from({ length: totalPages }, (_, i) => i + 1).filter(
    (p) => p === 1 || p === totalPages || Math.abs(p - currentPage) <= 1
  )
  return pages.reduce((acc, p, idx) => {
    if (idx > 0 && p - pages[idx - 1] > 1) acc.push('...')
    acc.push(p)
    return acc
  }, [])
}

// TruncatedText: potong teks panjang dengan ellipsis dan tampilkan teks
// lengkap lewat native title tooltip saat di-hover. Dipakai untuk konten di
// dalam kolom yang isinya bisa panjang (deskripsi, nama partner, dst).
// Wajib block-level (bukan inline-block) supaya text-overflow:ellipsis
// benar-benar dihitung dari max-width sendiri, bukan dari lebar konten.
export function TruncatedText({ value, maxWidth = 240, className = '' }) {
  if (value == null || value === '') return <span className="text-body-tertiary">&mdash;</span>
  const text = String(value)
  return (
    <div className={`text-truncate ${className}`} style={{ maxWidth, minWidth: 0 }} title={text}>
      {text}
    </div>
  )
}

// DataTable: tabel generik dengan search, sort per kolom, dan pagination --
// dipakai di seluruh halaman list (Chart of Accounts, Journal, Invoices,
// Role Management, User Management) supaya perilakunya konsisten.
//
// columns: [{ key, label, render?(row), searchValue?(row), sortValue?(row),
//             sortable = true, className?, cellClassName?, maxWidth? }]
// maxWidth (px): dipakai DUA kali -- di <td> itu sendiri (supaya kolom tabel
// dengan table-layout:auto tidak ikut melebar mengikuti konten panjang) dan
// di TruncatedText (ellipsis + hover title menampilkan teks lengkap). Tanpa
// constraint di level <td>, browser tetap melebarkan kolom sesuai konten
// penuh sebelum truncation sempat berlaku, sehingga hasilnya scroll
// horizontal, bukan potongan "...".
function DataTable({
  columns,
  data,
  rowKey = (row) => row.id,
  loading = false,
  emptyMessage = 'Tidak ada data.',
  searchable = true,
  searchPlaceholder = 'Cari...',
  pageSize = 10,
  toolbar,
}) {
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState({ key: null, dir: 'asc' })
  const [page, setPage] = useState(1)

  const filtered = useMemo(() => {
    if (!searchable || !search.trim()) return data
    const q = search.trim().toLowerCase()
    return data.filter((row) =>
      columns.some((col) => {
        if (col.searchValue) return String(col.searchValue(row) ?? '').toLowerCase().includes(q)
        if (col.render) return false
        return String(row[col.key] ?? '').toLowerCase().includes(q)
      })
    )
  }, [data, search, searchable, columns])

  const sorted = useMemo(() => {
    if (!sort.key) return filtered
    const col = columns.find((c) => c.key === sort.key)
    const accessor = col?.sortValue ?? ((row) => row[sort.key])
    return [...filtered].sort((a, b) => {
      const av = accessor(a)
      const bv = accessor(b)
      if (av == null && bv == null) return 0
      if (av == null) return -1
      if (bv == null) return 1
      if (av < bv) return sort.dir === 'asc' ? -1 : 1
      if (av > bv) return sort.dir === 'asc' ? 1 : -1
      return 0
    })
  }, [filtered, sort, columns])

  const totalPages = Math.max(1, Math.ceil(sorted.length / pageSize))
  const currentPage = Math.min(page, totalPages)
  const pageRows = sorted.slice((currentPage - 1) * pageSize, currentPage * pageSize)

  function toggleSort(col) {
    if (col.sortable === false) return
    setPage(1)
    setSort((prev) => {
      if (prev.key !== col.key) return { key: col.key, dir: 'asc' }
      if (prev.dir === 'asc') return { key: col.key, dir: 'desc' }
      return { key: null, dir: 'asc' }
    })
  }

  return (
    <div className="d-flex flex-column gap-2">
      {(searchable || toolbar) && (
        <div className="d-flex align-items-center justify-content-between flex-wrap gap-2">
          {searchable ? (
            <div className="edp-search-box edp-search-box-bordered" style={{ maxWidth: 320 }}>
              <i className="bi bi-search" />
              <input
                type="text"
                className="form-control form-control-sm border-0"
                placeholder={searchPlaceholder}
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value)
                  setPage(1)
                }}
              />
            </div>
          ) : (
            <div />
          )}
          {toolbar}
        </div>
      )}

      <div className="table-responsive">
        <table className="table table-hover align-middle mb-0">
          <thead>
            <tr>
              {columns.map((col) => (
                <th
                  key={col.key}
                  className={col.className}
                  onClick={() => toggleSort(col)}
                  style={{ cursor: col.sortable === false ? 'default' : 'pointer', userSelect: 'none', whiteSpace: 'nowrap' }}
                >
                  {col.label}
                  {col.sortable !== false && (
                    <i
                      className={`bi ms-1 small ${
                        sort.key === col.key ? (sort.dir === 'asc' ? 'bi-caret-up-fill' : 'bi-caret-down-fill') : 'bi-caret-down text-body-tertiary'
                      }`}
                    />
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {loading && (
              <tr>
                <td colSpan={columns.length} className="text-center text-secondary py-4">
                  Memuat...
                </td>
              </tr>
            )}
            {!loading && pageRows.length === 0 && (
              <tr>
                <td colSpan={columns.length} className="text-center text-secondary py-4">
                  {emptyMessage}
                </td>
              </tr>
            )}
            {!loading &&
              pageRows.map((row) => (
                <tr key={rowKey(row)}>
                  {columns.map((col) => (
                    <td
                      key={col.key}
                      className={col.cellClassName}
                      style={col.maxWidth ? { maxWidth: col.maxWidth, overflow: 'hidden' } : undefined}
                    >
                      {col.render
                        ? col.render(row)
                        : col.maxWidth
                          ? <TruncatedText value={row[col.key]} maxWidth={col.maxWidth} />
                          : row[col.key]}
                    </td>
                  ))}
                </tr>
              ))}
          </tbody>
        </table>
      </div>

      {!loading && sorted.length > 0 && (
        <div className="d-flex align-items-center justify-content-between flex-wrap gap-2">
          <span className="text-secondary small">
            Menampilkan {(currentPage - 1) * pageSize + 1}-{Math.min(currentPage * pageSize, sorted.length)} dari {sorted.length}
          </span>
          {totalPages > 1 && (
            <nav>
              <ul className="pagination pagination-sm mb-0">
                <li className={`page-item ${currentPage === 1 ? 'disabled' : ''}`}>
                  <button type="button" className="page-link" onClick={() => setPage((p) => Math.max(1, p - 1))}>
                    <i className="bi bi-chevron-left" />
                  </button>
                </li>
                {buildPageList(totalPages, currentPage).map((p, idx) =>
                  p === '...' ? (
                    <li key={`ellipsis-${idx}`} className="page-item disabled">
                      <span className="page-link">...</span>
                    </li>
                  ) : (
                    <li key={p} className={`page-item ${p === currentPage ? 'active' : ''}`}>
                      <button type="button" className="page-link" onClick={() => setPage(p)}>
                        {p}
                      </button>
                    </li>
                  )
                )}
                <li className={`page-item ${currentPage === totalPages ? 'disabled' : ''}`}>
                  <button type="button" className="page-link" onClick={() => setPage((p) => Math.min(totalPages, p + 1))}>
                    <i className="bi bi-chevron-right" />
                  </button>
                </li>
              </ul>
            </nav>
          )}
        </div>
      )}
    </div>
  )
}

export default DataTable
