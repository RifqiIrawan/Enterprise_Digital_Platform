package httpapi

import (
	"net/http"

	"github.com/enterprise-digital-platform/rbac-service/internal/model"
)

func (h *Handler) listModules(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, code, name, sort_order FROM modules ORDER BY sort_order ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar module")
		return
	}
	defer rows.Close()

	modules := []model.Module{}
	for rows.Next() {
		var m model.Module
		if err := rows.Scan(&m.ID, &m.Code, &m.Name, &m.SortOrder); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data module")
			return
		}
		modules = append(modules, m)
	}
	writeJSON(w, http.StatusOK, modules)
}

func (h *Handler) listMenus(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, module_id, parent_id, code, name, path, icon, sort_order, is_active
		 FROM menus WHERE is_active = true ORDER BY sort_order ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar menu")
		return
	}
	defer rows.Close()

	menus := []model.Menu{}
	for rows.Next() {
		var m model.Menu
		if err := rows.Scan(&m.ID, &m.ModuleID, &m.ParentID, &m.Code, &m.Name, &m.Path, &m.Icon, &m.SortOrder, &m.IsActive); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data menu")
			return
		}
		menus = append(menus, m)
	}
	writeJSON(w, http.StatusOK, menus)
}

type menuTreeItem struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Icon     string          `json:"icon"`
	Children []*menuTreeItem `json:"children"`
}

type moduleTreeItem struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Menus []*menuTreeItem `json:"menus"`
}

// menuTree mengembalikan menu yang boleh dilihat (can_view = true) oleh user,
// dikelompokkan per module, dipakai untuk merender sidebar. Super Admin
// (ditandai header X-Is-Super-Admin yang di-inject api-gateway dari klaim JWT)
// melihat seluruh menu aktif tanpa perlu row di user_roles/role_menu_permissions.
func (h *Handler) menuTree(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id wajib diisi")
		return
	}
	isSuperAdmin := r.Header.Get("X-Is-Super-Admin") == "true"

	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
	var err error

	if isSuperAdmin {
		rows, err = h.pool.Query(r.Context(), `
			SELECT m.id, m.parent_id, m.name, m.path, m.icon, m.sort_order,
			       mod.id, mod.name, mod.sort_order
			FROM menus m
			JOIN modules mod ON mod.id = m.module_id
			WHERE m.is_active = true
			ORDER BY mod.sort_order ASC, m.sort_order ASC`)
	} else {
		rows, err = h.pool.Query(r.Context(), `
			SELECT m.id, m.parent_id, m.name, m.path, m.icon, m.sort_order,
			       mod.id, mod.name, mod.sort_order
			FROM menus m
			JOIN modules mod ON mod.id = m.module_id
			WHERE m.is_active = true
			  AND m.id IN (
			    SELECT DISTINCT rmp.menu_id
			    FROM user_roles ur
			    JOIN role_menu_permissions rmp ON rmp.role_id = ur.role_id
			    WHERE ur.user_id = $1 AND rmp.can_view = true
			  )
			ORDER BY mod.sort_order ASC, m.sort_order ASC`, userID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat menu-tree")
		return
	}
	defer rows.Close()

	moduleOrder := []string{}
	moduleByID := map[string]*moduleTreeItem{}
	nodeByID := map[string]*menuTreeItem{}
	parentOf := map[string]*string{}
	moduleOfNode := map[string]string{}

	for rows.Next() {
		var menuID string
		var parentID *string
		var name, path, icon string
		var sortOrder int
		var modID, modName string
		var modSortOrder int
		if err := rows.Scan(&menuID, &parentID, &name, &path, &icon, &sortOrder, &modID, &modName, &modSortOrder); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca menu-tree")
			return
		}
		if _, ok := moduleByID[modID]; !ok {
			moduleByID[modID] = &moduleTreeItem{ID: modID, Name: modName, Menus: []*menuTreeItem{}}
			moduleOrder = append(moduleOrder, modID)
		}
		node := &menuTreeItem{ID: menuID, Name: name, Path: path, Icon: icon, Children: []*menuTreeItem{}}
		nodeByID[menuID] = node
		parentOf[menuID] = parentID
		moduleOfNode[menuID] = modID
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca menu-tree")
		return
	}

	for menuID, node := range nodeByID {
		parentID := parentOf[menuID]
		if parentID != nil {
			if parent, ok := nodeByID[*parentID]; ok {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		modID := moduleOfNode[menuID]
		moduleByID[modID].Menus = append(moduleByID[modID].Menus, node)
	}

	tree := make([]*moduleTreeItem, 0, len(moduleOrder))
	for _, modID := range moduleOrder {
		tree = append(tree, moduleByID[modID])
	}
	writeJSON(w, http.StatusOK, tree)
}
