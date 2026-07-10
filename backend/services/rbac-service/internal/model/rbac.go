package model

import "time"

type Module struct {
	ID        string `json:"id" db:"id"`
	Code      string `json:"code" db:"code"`
	Name      string `json:"name" db:"name"`
	SortOrder int    `json:"sort_order" db:"sort_order"`
}

type Menu struct {
	ID        string  `json:"id" db:"id"`
	ModuleID  string  `json:"module_id" db:"module_id"`
	ParentID  *string `json:"parent_id" db:"parent_id"`
	Code      string  `json:"code" db:"code"`
	Name      string  `json:"name" db:"name"`
	Path      string  `json:"path" db:"path"`
	Icon      string  `json:"icon" db:"icon"`
	SortOrder int     `json:"sort_order" db:"sort_order"`
	IsActive  bool    `json:"is_active" db:"is_active"`
}

type Role struct {
	ID          string    `json:"id" db:"id"`
	CompanyID   *string   `json:"company_id" db:"company_id"` // nil = role template global/system
	Code        string    `json:"code" db:"code"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	IsSystem    bool      `json:"is_system" db:"is_system"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// MenuActions adalah kombinasi aksi yang diizinkan pada sebuah menu.
// CanView saja true = view-only; semua true = full access.
type MenuActions struct {
	CanView    bool `json:"can_view" db:"can_view"`
	CanCreate  bool `json:"can_create" db:"can_create"`
	CanUpdate  bool `json:"can_update" db:"can_update"`
	CanDelete  bool `json:"can_delete" db:"can_delete"`
	CanApprove bool `json:"can_approve" db:"can_approve"`
	CanExport  bool `json:"can_export" db:"can_export"`
}

// Or menggabungkan dua MenuActions secara OR per kolom (dipakai saat user
// punya beberapa role yang menyentuh menu yang sama).
func (a MenuActions) Or(b MenuActions) MenuActions {
	return MenuActions{
		CanView:    a.CanView || b.CanView,
		CanCreate:  a.CanCreate || b.CanCreate,
		CanUpdate:  a.CanUpdate || b.CanUpdate,
		CanDelete:  a.CanDelete || b.CanDelete,
		CanApprove: a.CanApprove || b.CanApprove,
		CanExport:  a.CanExport || b.CanExport,
	}
}

type RoleMenuPermission struct {
	ID          string    `json:"id" db:"id"`
	RoleID      string    `json:"role_id" db:"role_id"`
	MenuID      string    `json:"menu_id" db:"menu_id"`
	MenuActions           // flattened: can_view, can_create, can_update, can_delete, can_approve, can_export
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// UserRole adalah penugasan role ke user pada scope company/branch/department
// tertentu. BranchID/DepartmentID nil berarti berlaku untuk seluruh
// branch/department dalam scope di atasnya.
type UserRole struct {
	ID           string     `json:"id" db:"id"`
	UserID       string     `json:"user_id" db:"user_id"`
	RoleID       string     `json:"role_id" db:"role_id"`
	CompanyID    string     `json:"company_id" db:"company_id"`
	BranchID     *string    `json:"branch_id" db:"branch_id"`
	DepartmentID *string    `json:"department_id" db:"department_id"`
	AssignedBy   *string    `json:"assigned_by" db:"assigned_by"`
	ValidFrom    time.Time  `json:"valid_from" db:"valid_from"`
	ValidTo      *time.Time `json:"valid_to" db:"valid_to"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// UserMenuPermissionOverride menimpa hasil role_menu_permissions pada scope
// company/branch/department/menu yang sama persis untuk satu user tertentu.
type UserMenuPermissionOverride struct {
	ID           string    `json:"id" db:"id"`
	UserID       string    `json:"user_id" db:"user_id"`
	CompanyID    string    `json:"company_id" db:"company_id"`
	BranchID     *string   `json:"branch_id" db:"branch_id"`
	DepartmentID *string   `json:"department_id" db:"department_id"`
	MenuID       string    `json:"menu_id" db:"menu_id"`
	MenuActions            // flattened: can_view, can_create, can_update, can_delete, can_approve, can_export
	CreatedBy    *string   `json:"created_by" db:"created_by"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
