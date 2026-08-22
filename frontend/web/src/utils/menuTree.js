// Logika pencocokan path untuk accordion sidebar. Dipisah dari Sidebar.jsx
// supaya bisa diuji langsung terhadap keluaran asli GET /api/rbac/menu-tree
// tanpa perlu me-render React.

// matchesPath: pencocokan berbatas segmen, bukan startsWith polos. Tanpa batas
// itu, "/admin/roles" akan ikut mengklaim "/admin/roles-lain" seandainya nanti
// ada path seperti itu. Prefix tetap diperlukan (bukan sekadar sama persis)
// karena ada rute turunan yang tidak punya menu sendiri, mis.
// "/admin/roles/new" dan "/admin/roles/:id/permissions".
export function matchesPath(menuPath, pathname) {
  if (!menuPath) return false
  return pathname === menuPath || pathname.startsWith(`${menuPath}/`)
}

export function moduleContainsPath(mod, pathname) {
  const walk = (menus) =>
    menus.some((menu) => matchesPath(menu.path, pathname) || (menu.children?.length > 0 && walk(menu.children)))
  return walk(mod.menus ?? [])
}

// findModuleIdForPath mengembalikan id modul yang memuat halaman aktif, atau
// null kalau tidak ada (mis. Dashboard, yang memang di luar accordion).
export function findModuleIdForPath(moduleTree, pathname) {
  return moduleTree?.find((mod) => moduleContainsPath(mod, pathname))?.id ?? null
}
