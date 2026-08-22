package authz

import (
	"fmt"
	"strings"
)

// Kebijakan hak akses: satu tabel yang memetakan endpoint API ke MENU beserta
// aksi yang dibutuhkannya. Gateway adalah satu-satunya titik lewat request dari
// browser, jadi di sinilah penegakannya -- bukan disalin ke 20 service.
//
// Konsekuensi yang perlu diingat: service modul TIDAK memeriksa apa pun
// sendiri. Siapa pun yang bisa menghubungi port service secara langsung (bukan
// lewat gateway) tetap dapat memanggilnya. Di deployment K8s/compose yang ada,
// hanya gateway yang di-expose keluar, dan panggilan antar-service memang
// sengaja langsung ke service tujuan (mis. sales-service -> warehouse-service)
// bukan memutar lewat gateway; menaruh penegakan di sini karena itu tidak
// memutus jalur tersebut.
//
// TIGA ATURAN YANG MEMBENTUK TABEL DI BAWAH:
//
//  1. Endpoint yang menyajikan DATA ACUAN boleh diakses dari halaman mana pun
//     yang benar-benar memakainya. GET /api/warehouse/products bukan hanya
//     milik halaman Master Barang -- form Work Order, Inspeksi QC, Stock
//     Opname dan lainnya mengisi dropdown-nya dari situ. Daftar menu di tiap
//     baris viewAny(...) diambil dari pemakaian nyata di frontend, bukan
//     dikira-kira; halaman baru yang memakai data acuan berarti menunya harus
//     ditambahkan di sini juga.
//
//  2. Aksi yang MENYENTUH BUKU BESAR ATAU STOK butuh `approve`, bukan
//     `update`. Posting jurnal/invoice/payroll, menerima PO, fulfill SO,
//     posting opname, konfirmasi transfer, menyelesaikan Work Order, mengirim
//     order e-commerce -- semuanya menghasilkan angka yang tidak bisa ditarik
//     kembali begitu saja. Perpindahan status yang tidak menghasilkan angka
//     (submit, kirim penawaran, mulai produksi, batal, tutup tiket) cukup
//     `update`.
//
//  3. Endpoint yang TIDAK ADA di tabel ini DITOLAK, bukan diteruskan diam-diam.
//     Endpoint baru yang lupa didaftarkan akan langsung terlihat sebagai 403
//     saat dicoba -- kebalikannya (default meneruskan) membuat endpoint baru
//     berdiri tanpa penjagaan sama sekali dan tidak ada yang tahu sampai ada
//     yang mencarinya. TestPolicyCoversEveryRegisteredRoute menjaga janji ini
//     terhadap route yang benar-benar terdaftar di seluruh service.

// Action adalah kolom hak di rbac-service (can_view, can_create, ...).
type Action string

const (
	View    Action = "view"
	Create  Action = "create"
	Update  Action = "update"
	Delete  Action = "delete"
	Approve Action = "approve"
	Export  Action = "export"
)

type kind int

const (
	// kindPermission: butuh SALAH SATU dari needs.
	kindPermission kind = iota
	// kindAuthenticated: token valid sudah cukup. Dipakai untuk hal yang memang
	// milik setiap user (ganti password sendiri, daftar company untuk switcher,
	// menu-tree untuk sidebar) -- menuntut hak menu di situ membuat aplikasi
	// tidak bisa digambar sama sekali.
	kindAuthenticated
	// kindInternal: tidak boleh lewat gateway sama sekali. Endpoint yang hanya
	// dipakai service lain secara langsung.
	kindInternal
)

// Need adalah satu pasangan menu+aksi yang bisa memenuhi sebuah rule.
type Need struct {
	Menu   string
	Action Action
}

// Requirement adalah syarat sebuah endpoint. selfParam mengisi kasus khusus
// "boleh kalau menanyakan diri sendiri": beberapa endpoint rbac menerima
// user_id, dan setiap user berhak menanyakan haknya sendiri tanpa harus punya
// hak User Management.
type Requirement struct {
	kind      kind
	needs     []Need
	selfParam string
}

type Rule struct {
	Method  string
	Pattern string // segmen "*" cocok dengan satu segmen apa pun
	Req     Requirement
}

func need(menu string, action Action) Requirement {
	return Requirement{kind: kindPermission, needs: []Need{{Menu: menu, Action: action}}}
}

// viewAny: hak lihat pada salah satu menu di daftar sudah cukup. Dipakai untuk
// data acuan (aturan 1 di atas).
func viewAny(menus ...string) Requirement {
	needs := make([]Need, 0, len(menus))
	for _, m := range menus {
		needs = append(needs, Need{Menu: m, Action: View})
	}
	return Requirement{kind: kindPermission, needs: needs}
}

func anyOf(reqs ...Requirement) Requirement {
	var needs []Need
	for _, r := range reqs {
		needs = append(needs, r.needs...)
	}
	return Requirement{kind: kindPermission, needs: needs}
}

func authenticated() Requirement { return Requirement{kind: kindAuthenticated} }

func internalOnly() Requirement { return Requirement{kind: kindInternal} }

// selfOr membolehkan request yang query param `param`-nya berisi id user
// pemanggil sendiri; selain itu berlaku syarat r.
func selfOr(param string, r Requirement) Requirement {
	r.selfParam = param
	return r
}

// Daftar menu pemakai data acuan, dinamai supaya pemakaiannya di beberapa baris
// tidak berubah di satu tempat dan tertinggal di tempat lain.
var (
	// GET /api/warehouse/products
	productConsumers = []string{
		"/warehouse/products", "/warehouse/stock", "/warehouse/opname", "/warehouse/transfers",
		"/production/bom", "/production/work-orders", "/production/schedule",
		"/qc/inspections", "/qc/standards", "/ecommerce/orders",
	}
	// GET /api/warehouse/warehouses
	warehouseConsumers = []string{
		"/warehouse/warehouses", "/warehouse/stock", "/warehouse/opname", "/warehouse/transfers",
		"/production/work-orders", "/purchasing/orders", "/sales/orders",
		"/asset/register", "/iot/devices", "/ecommerce/orders",
	}
	// GET /api/finance/accounts
	accountConsumers = []string{
		"/finance/accounts", "/finance/journal", "/finance/invoices",
		"/hr/payroll", "/purchasing/orders", "/sales/orders", "/project/projects",
	}
	// GET /api/hr/employees
	employeeConsumers = []string{
		"/hr/employees", "/hr/attendance", "/hr/leave", "/hr/overtime", "/hr/kpi-reviews",
		"/project/projects", "/project/tasks", "/project/timesheets",
	}
)

// Rules sengaja ditulis satu baris per endpoint, bukan pola pintar per modul:
// yang dijaga di sini adalah keputusan per endpoint, dan pola yang menebak
// otomatis akan diam-diam menjaga endpoint baru dengan aturan yang salah.
var Rules = []Rule{
	// ---------- auth-service ----------
	// login & refresh tidak sampai ke sini (publicRoutes di gateway.go).
	{"POST", "/api/auth/logout", authenticated()},
	{"POST", "/api/auth/change-password", authenticated()},
	{"GET", "/api/auth/users", need("/admin/users", View)},
	{"POST", "/api/auth/users", need("/admin/users", Create)},
	{"PUT", "/api/auth/users/*", need("/admin/users", Update)},
	{"POST", "/api/auth/users/*/reset-password", need("/admin/users", Update)},

	// ---------- company-service ----------
	// Daftar company & branch dipakai SETIAP user lewat switcher di Topbar,
	// jadi membacanya bukan hak istimewa admin -- yang dijaga mengubahnya.
	{"GET", "/api/company/companies", authenticated()},
	{"GET", "/api/company/companies/*", authenticated()},
	{"GET", "/api/company/companies/*/branches", authenticated()},
	{"GET", "/api/company/companies/*/departments", authenticated()},
	{"POST", "/api/company/companies", need("/admin/companies", Create)},
	{"PUT", "/api/company/companies/*", need("/admin/companies", Update)},
	{"POST", "/api/company/companies/*/branches", need("/admin/branches", Create)},
	{"PUT", "/api/company/companies/*/branches/*", need("/admin/branches", Update)},
	{"DELETE", "/api/company/companies/*/branches/*", need("/admin/branches", Delete)},
	{"POST", "/api/company/companies/*/departments", need("/admin/departments", Create)},
	{"PUT", "/api/company/companies/*/departments/*", need("/admin/departments", Update)},
	{"DELETE", "/api/company/companies/*/departments/*", need("/admin/departments", Delete)},

	// ---------- rbac-service ----------
	{"GET", "/api/rbac/menu-tree", authenticated()},
	{"GET", "/api/rbac/modules", authenticated()},
	{"GET", "/api/rbac/menus", viewAny("/admin/roles", "/admin/users")},
	{"GET", "/api/rbac/roles", viewAny("/admin/roles", "/admin/users")},
	{"POST", "/api/rbac/roles", need("/admin/roles", Create)},
	{"PUT", "/api/rbac/roles/*", need("/admin/roles", Update)},
	{"DELETE", "/api/rbac/roles/*", need("/admin/roles", Delete)},
	{"GET", "/api/rbac/roles/*/permissions", need("/admin/roles", View)},
	// Boleh membuat role tapi tidak boleh mengisi haknya menghasilkan role
	// kosong yang tidak berguna -- halaman "Role baru" memang melakukan
	// keduanya berurutan. Karena itu `create` ikut diterima di sini,
	// satu-satunya tempat di tabel ini yang begitu.
	{"PUT", "/api/rbac/roles/*/permissions", anyOf(need("/admin/roles", Update), need("/admin/roles", Create))},
	{"GET", "/api/rbac/user-permissions", selfOr("user_id", need("/admin/users", View))},
	{"GET", "/api/rbac/user-overrides", selfOr("user_id", need("/admin/users", View))},
	{"PUT", "/api/rbac/user-overrides", need("/admin/users", Update)},
	{"DELETE", "/api/rbac/user-overrides/*", need("/admin/users", Update)},
	{"GET", "/api/rbac/user-roles", selfOr("user_id", need("/admin/users", View))},
	{"POST", "/api/rbac/user-roles", need("/admin/users", Create)},
	{"DELETE", "/api/rbac/user-roles/*", need("/admin/users", Delete)},
	// Sumber kebenaran yang dipakai gateway ini sendiri. Kalau browser boleh
	// memanggilnya, siapa pun bisa membaca hak orang lain di company mana pun.
	{"GET", "/api/rbac/access", internalOnly()},

	// ---------- audit-service ----------
	{"GET", "/api/audit/audit-logs", need("/admin/audit-logs", View)},

	// ---------- finance-service ----------
	{"GET", "/api/finance/accounts", viewAny(accountConsumers...)},
	{"POST", "/api/finance/accounts", need("/finance/accounts", Create)},
	{"PUT", "/api/finance/accounts/*", need("/finance/accounts", Update)},
	{"GET", "/api/finance/journal-entries", need("/finance/journal", View)},
	{"GET", "/api/finance/journal-entries/*", need("/finance/journal", View)},
	{"POST", "/api/finance/journal-entries", need("/finance/journal", Create)},
	{"POST", "/api/finance/journal-entries/*/post", need("/finance/journal", Approve)},
	{"GET", "/api/finance/invoices", need("/finance/invoices", View)},
	{"GET", "/api/finance/invoices/*", need("/finance/invoices", View)},
	{"POST", "/api/finance/invoices", need("/finance/invoices", Create)},
	{"POST", "/api/finance/invoices/*/post", need("/finance/invoices", Approve)},
	{"GET", "/api/finance/ar-ap-summary", need("/finance/ar-ap", View)},

	// ---------- hr-service ----------
	{"GET", "/api/hr/employees", viewAny(employeeConsumers...)},
	{"GET", "/api/hr/employees/*", need("/hr/employees", View)},
	{"POST", "/api/hr/employees", need("/hr/employees", Create)},
	{"PUT", "/api/hr/employees/*", need("/hr/employees", Update)},
	{"GET", "/api/hr/attendance", need("/hr/attendance", View)},
	{"POST", "/api/hr/attendance", need("/hr/attendance", Create)},
	{"PUT", "/api/hr/attendance/*", need("/hr/attendance", Update)},
	{"GET", "/api/hr/payroll-runs", need("/hr/payroll", View)},
	{"GET", "/api/hr/payroll-runs/*", need("/hr/payroll", View)},
	{"POST", "/api/hr/payroll-runs", need("/hr/payroll", Create)},
	{"POST", "/api/hr/payroll-runs/*/post", need("/hr/payroll", Approve)},
	{"GET", "/api/hr/leave-requests", need("/hr/leave", View)},
	{"POST", "/api/hr/leave-requests", need("/hr/leave", Create)},
	{"PUT", "/api/hr/leave-requests/*", need("/hr/leave", Update)},
	{"POST", "/api/hr/leave-requests/*/submit", need("/hr/leave", Update)},
	{"POST", "/api/hr/leave-requests/*/cancel", need("/hr/leave", Update)},
	{"POST", "/api/hr/leave-requests/*/approve", need("/hr/leave", Approve)},
	{"POST", "/api/hr/leave-requests/*/reject", need("/hr/leave", Approve)},
	{"GET", "/api/hr/overtime", need("/hr/overtime", View)},
	{"POST", "/api/hr/overtime", need("/hr/overtime", Create)},
	{"PUT", "/api/hr/overtime/*", need("/hr/overtime", Update)},
	{"POST", "/api/hr/overtime/*/approve", need("/hr/overtime", Approve)},
	{"POST", "/api/hr/overtime/*/reject", need("/hr/overtime", Approve)},
	{"GET", "/api/hr/holidays", need("/hr/holidays", View)},
	{"POST", "/api/hr/holidays", need("/hr/holidays", Create)},
	{"DELETE", "/api/hr/holidays/*", need("/hr/holidays", Delete)},
	{"GET", "/api/hr/leave-quotas", need("/hr/leave-quota", View)},
	{"PUT", "/api/hr/leave-quotas", need("/hr/leave-quota", Update)},
	{"GET", "/api/hr/kpi-indicators", need("/hr/kpi-indicators", View)},
	{"POST", "/api/hr/kpi-indicators", need("/hr/kpi-indicators", Create)},
	{"PUT", "/api/hr/kpi-indicators/*", need("/hr/kpi-indicators", Update)},
	{"DELETE", "/api/hr/kpi-indicators/*", need("/hr/kpi-indicators", Delete)},
	{"GET", "/api/hr/kpi-reviews", need("/hr/kpi-reviews", View)},
	{"GET", "/api/hr/kpi-reviews/*", need("/hr/kpi-reviews", View)},
	{"POST", "/api/hr/kpi-reviews", need("/hr/kpi-reviews", Create)},
	{"PUT", "/api/hr/kpi-reviews/*/scores", need("/hr/kpi-reviews", Update)},
	{"POST", "/api/hr/kpi-reviews/*/submit", need("/hr/kpi-reviews", Update)},
	{"POST", "/api/hr/kpi-reviews/*/approve", need("/hr/kpi-reviews", Approve)},
	{"POST", "/api/hr/kpi-reviews/*/reject", need("/hr/kpi-reviews", Approve)},

	// ---------- sales-service ----------
	{"GET", "/api/sales/customers", viewAny("/sales/customers", "/sales/quotations", "/sales/orders")},
	{"POST", "/api/sales/customers", need("/sales/customers", Create)},
	{"PUT", "/api/sales/customers/*", need("/sales/customers", Update)},
	{"GET", "/api/sales/quotations", need("/sales/quotations", View)},
	{"GET", "/api/sales/quotations/*", need("/sales/quotations", View)},
	{"POST", "/api/sales/quotations", need("/sales/quotations", Create)},
	{"POST", "/api/sales/quotations/*/send", need("/sales/quotations", Update)},
	{"POST", "/api/sales/quotations/*/convert", need("/sales/quotations", Update)},
	{"POST", "/api/sales/quotations/*/accept", need("/sales/quotations", Approve)},
	{"POST", "/api/sales/quotations/*/reject", need("/sales/quotations", Approve)},
	{"GET", "/api/sales/sales-orders", need("/sales/orders", View)},
	{"GET", "/api/sales/sales-orders/*", need("/sales/orders", View)},
	{"POST", "/api/sales/sales-orders", need("/sales/orders", Create)},
	{"POST", "/api/sales/sales-orders/*/confirm", need("/sales/orders", Update)},
	{"POST", "/api/sales/sales-orders/*/fulfill", need("/sales/orders", Approve)},
	{"POST", "/api/sales/sales-orders/*/invoice", need("/sales/orders", Approve)},

	// ---------- purchasing-service ----------
	{"GET", "/api/purchasing/suppliers", viewAny("/purchasing/suppliers", "/purchasing/requisitions", "/purchasing/orders")},
	{"POST", "/api/purchasing/suppliers", need("/purchasing/suppliers", Create)},
	{"PUT", "/api/purchasing/suppliers/*", need("/purchasing/suppliers", Update)},
	{"GET", "/api/purchasing/requisitions", need("/purchasing/requisitions", View)},
	{"GET", "/api/purchasing/requisitions/*", need("/purchasing/requisitions", View)},
	{"POST", "/api/purchasing/requisitions", need("/purchasing/requisitions", Create)},
	{"POST", "/api/purchasing/requisitions/*/submit", need("/purchasing/requisitions", Update)},
	{"POST", "/api/purchasing/requisitions/*/convert", need("/purchasing/requisitions", Update)},
	{"POST", "/api/purchasing/requisitions/*/approve", need("/purchasing/requisitions", Approve)},
	{"POST", "/api/purchasing/requisitions/*/reject", need("/purchasing/requisitions", Approve)},
	{"GET", "/api/purchasing/purchase-orders", viewAny("/purchasing/orders", "/qc/inspections")},
	{"GET", "/api/purchasing/purchase-orders/*", need("/purchasing/orders", View)},
	{"POST", "/api/purchasing/purchase-orders", need("/purchasing/orders", Create)},
	{"POST", "/api/purchasing/purchase-orders/*/confirm", need("/purchasing/orders", Update)},
	{"POST", "/api/purchasing/purchase-orders/*/receive", need("/purchasing/orders", Approve)},
	{"POST", "/api/purchasing/purchase-orders/*/invoice", need("/purchasing/orders", Approve)},

	// ---------- warehouse-service ----------
	{"GET", "/api/warehouse/products", viewAny(productConsumers...)},
	{"POST", "/api/warehouse/products", need("/warehouse/products", Create)},
	{"PUT", "/api/warehouse/products/*", need("/warehouse/products", Update)},
	{"GET", "/api/warehouse/warehouses", viewAny(warehouseConsumers...)},
	{"POST", "/api/warehouse/warehouses", need("/warehouse/warehouses", Create)},
	{"PUT", "/api/warehouse/warehouses/*", need("/warehouse/warehouses", Update)},
	{"GET", "/api/warehouse/stock", need("/warehouse/stock", View)},
	{"GET", "/api/warehouse/stock-movements", need("/warehouse/stock", View)},
	{"POST", "/api/warehouse/stock-movements", need("/warehouse/stock", Create)},
	// Dipanggil sales/purchasing/production/ecommerce-service LANGSUNG ke
	// warehouse-service saat dokumen mereka menggerakkan stok, tidak pernah
	// dari browser. Membiarkannya lewat gateway berarti siapa pun yang boleh
	// membuat satu stock movement bisa memasukkan sebatch apa pun sekaligus.
	{"POST", "/api/warehouse/stock-movements/batch", internalOnly()},
	{"GET", "/api/warehouse/stock-transfers", need("/warehouse/transfers", View)},
	{"GET", "/api/warehouse/stock-transfers/*", need("/warehouse/transfers", View)},
	{"POST", "/api/warehouse/stock-transfers", need("/warehouse/transfers", Create)},
	{"POST", "/api/warehouse/stock-transfers/*/confirm", need("/warehouse/transfers", Approve)},
	{"GET", "/api/warehouse/stock-opnames", need("/warehouse/opname", View)},
	{"GET", "/api/warehouse/stock-opnames/*", need("/warehouse/opname", View)},
	{"POST", "/api/warehouse/stock-opnames", need("/warehouse/opname", Create)},
	{"POST", "/api/warehouse/stock-opnames/*/post", need("/warehouse/opname", Approve)},

	// ---------- production-service ----------
	{"GET", "/api/production/boms", viewAny("/production/bom", "/production/work-orders", "/production/schedule")},
	{"GET", "/api/production/boms/*", need("/production/bom", View)},
	{"POST", "/api/production/boms", need("/production/bom", Create)},
	{"PUT", "/api/production/boms/*", need("/production/bom", Update)},
	{"GET", "/api/production/work-orders", viewAny("/production/work-orders", "/production/schedule", "/qc/inspections")},
	{"GET", "/api/production/work-orders/*", need("/production/work-orders", View)},
	{"POST", "/api/production/work-orders", need("/production/work-orders", Create)},
	{"POST", "/api/production/work-orders/*/start", need("/production/work-orders", Update)},
	{"POST", "/api/production/work-orders/*/complete", need("/production/work-orders", Approve)},

	// ---------- qc-service ----------
	{"GET", "/api/qc/standards", viewAny("/qc/standards", "/qc/inspections")},
	{"POST", "/api/qc/standards", need("/qc/standards", Create)},
	{"PUT", "/api/qc/standards/*", need("/qc/standards", Update)},
	{"GET", "/api/qc/inspections", need("/qc/inspections", View)},
	{"GET", "/api/qc/inspections/*", need("/qc/inspections", View)},
	{"POST", "/api/qc/inspections", need("/qc/inspections", Create)},

	// ---------- asset-service ----------
	{"GET", "/api/asset/assets", viewAny("/asset/register", "/asset/maintenance")},
	{"POST", "/api/asset/assets", need("/asset/register", Create)},
	{"PUT", "/api/asset/assets/*", need("/asset/register", Update)},
	{"GET", "/api/asset/maintenance-schedules", need("/asset/maintenance", View)},
	{"POST", "/api/asset/maintenance-schedules", need("/asset/maintenance", Create)},
	{"POST", "/api/asset/maintenance-schedules/*/complete", need("/asset/maintenance", Update)},
	{"POST", "/api/asset/maintenance-schedules/*/cancel", need("/asset/maintenance", Update)},

	// ---------- ai-bi-service ----------
	{"GET", "/api/ai-bi/dashboards/summary", need("/ai-bi/dashboards", View)},
	{"GET", "/api/ai-bi/forecasting/summary", need("/ai-bi/forecasting", View)},
	{"GET", "/api/ai-bi/anomaly-detection/scan", need("/ai-bi/anomaly-detection", View)},

	// ---------- iot-service ----------
	{"GET", "/api/iot/devices", viewAny("/iot/devices", "/iot/readings")},
	{"GET", "/api/iot/devices/*", need("/iot/devices", View)},
	{"POST", "/api/iot/devices", need("/iot/devices", Create)},
	{"PUT", "/api/iot/devices/*", need("/iot/devices", Update)},
	{"GET", "/api/iot/readings", need("/iot/readings", View)},
	{"GET", "/api/iot/alerts", need("/iot/alerts", View)},
	{"POST", "/api/iot/alerts/*/acknowledge", need("/iot/alerts", Update)},
	{"POST", "/api/iot/alerts/*/resolve", need("/iot/alerts", Update)},

	// ---------- dw-service ----------
	// Seluruh /analytics/* menyuplai satu halaman: BI Dashboards.
	{"GET", "/api/dw/analytics/*", need("/ai-bi/dashboards", View)},
	{"GET", "/api/dw/sync/status", need("/dw/sync-status", View)},
	{"POST", "/api/dw/sync", need("/dw/sync-status", Create)},

	// ---------- crm-service ----------
	{"GET", "/api/crm/leads", viewAny("/crm/leads", "/crm/activities")},
	{"POST", "/api/crm/leads", need("/crm/leads", Create)},
	{"PUT", "/api/crm/leads/*", need("/crm/leads", Update)},
	{"POST", "/api/crm/leads/*/convert", need("/crm/leads", Update)},
	{"GET", "/api/crm/accounts", viewAny("/crm/accounts", "/crm/contacts", "/crm/opportunities", "/crm/activities")},
	{"POST", "/api/crm/accounts", need("/crm/accounts", Create)},
	{"PUT", "/api/crm/accounts/*", need("/crm/accounts", Update)},
	{"GET", "/api/crm/contacts", viewAny("/crm/contacts", "/crm/opportunities", "/crm/activities")},
	{"POST", "/api/crm/contacts", need("/crm/contacts", Create)},
	{"PUT", "/api/crm/contacts/*", need("/crm/contacts", Update)},
	{"GET", "/api/crm/opportunities", viewAny("/crm/opportunities", "/crm/activities")},
	{"POST", "/api/crm/opportunities", need("/crm/opportunities", Create)},
	{"PUT", "/api/crm/opportunities/*", need("/crm/opportunities", Update)},
	{"POST", "/api/crm/opportunities/*/win", need("/crm/opportunities", Update)},
	{"POST", "/api/crm/opportunities/*/lose", need("/crm/opportunities", Update)},
	{"GET", "/api/crm/activities", need("/crm/activities", View)},
	{"POST", "/api/crm/activities", need("/crm/activities", Create)},
	{"PUT", "/api/crm/activities/*", need("/crm/activities", Update)},

	// ---------- ticketing-service ----------
	{"GET", "/api/ticketing/categories", viewAny("/ticketing/categories", "/ticketing/tickets")},
	{"POST", "/api/ticketing/categories", need("/ticketing/categories", Create)},
	{"PUT", "/api/ticketing/categories/*", need("/ticketing/categories", Update)},
	{"GET", "/api/ticketing/tickets", viewAny("/ticketing/tickets", "/ticketing/comments")},
	{"POST", "/api/ticketing/tickets", need("/ticketing/tickets", Create)},
	{"PUT", "/api/ticketing/tickets/*", need("/ticketing/tickets", Update)},
	{"POST", "/api/ticketing/tickets/*/close", need("/ticketing/tickets", Update)},
	{"POST", "/api/ticketing/tickets/*/reopen", need("/ticketing/tickets", Update)},
	{"GET", "/api/ticketing/comments", need("/ticketing/comments", View)},
	{"POST", "/api/ticketing/comments", need("/ticketing/comments", Create)},

	// ---------- ecommerce-service ----------
	{"GET", "/api/ecommerce/orders", need("/ecommerce/orders", View)},
	{"GET", "/api/ecommerce/orders/*", need("/ecommerce/orders", View)},
	{"POST", "/api/ecommerce/orders", need("/ecommerce/orders", Create)},
	{"PUT", "/api/ecommerce/orders/*", need("/ecommerce/orders", Update)},
	{"POST", "/api/ecommerce/orders/*/pay", need("/ecommerce/orders", Update)},
	// ship mengurangi stok gudang lewat warehouse-service; deliver & cancel tidak.
	{"POST", "/api/ecommerce/orders/*/ship", need("/ecommerce/orders", Approve)},
	{"POST", "/api/ecommerce/orders/*/deliver", need("/ecommerce/orders", Update)},
	{"POST", "/api/ecommerce/orders/*/cancel", need("/ecommerce/orders", Update)},

	// ---------- fleet-service ----------
	{"GET", "/api/fleet/vehicles", viewAny("/fleet/vehicles", "/fleet/delivery-orders")},
	{"POST", "/api/fleet/vehicles", need("/fleet/vehicles", Create)},
	{"PUT", "/api/fleet/vehicles/*", need("/fleet/vehicles", Update)},
	{"GET", "/api/fleet/drivers", viewAny("/fleet/drivers", "/fleet/delivery-orders")},
	{"POST", "/api/fleet/drivers", need("/fleet/drivers", Create)},
	{"PUT", "/api/fleet/drivers/*", need("/fleet/drivers", Update)},
	{"GET", "/api/fleet/delivery-orders", need("/fleet/delivery-orders", View)},
	{"GET", "/api/fleet/delivery-orders/*", need("/fleet/delivery-orders", View)},
	{"POST", "/api/fleet/delivery-orders", need("/fleet/delivery-orders", Create)},
	{"PUT", "/api/fleet/delivery-orders/*", need("/fleet/delivery-orders", Update)},
	{"POST", "/api/fleet/delivery-orders/*/dispatch", need("/fleet/delivery-orders", Update)},
	{"POST", "/api/fleet/delivery-orders/*/deliver", need("/fleet/delivery-orders", Update)},
	{"POST", "/api/fleet/delivery-orders/*/cancel", need("/fleet/delivery-orders", Update)},

	// ---------- project-service ----------
	{"GET", "/api/project/projects", viewAny("/project/projects", "/project/tasks", "/project/timesheets")},
	{"GET", "/api/project/projects/*", need("/project/projects", View)},
	{"POST", "/api/project/projects", need("/project/projects", Create)},
	{"PUT", "/api/project/projects/*", need("/project/projects", Update)},
	{"POST", "/api/project/projects/*/activate", need("/project/projects", Update)},
	{"POST", "/api/project/projects/*/hold", need("/project/projects", Update)},
	{"POST", "/api/project/projects/*/complete", need("/project/projects", Update)},
	{"POST", "/api/project/projects/*/cancel", need("/project/projects", Update)},
	// post-cost membukukan biaya proyek ke jurnal GL.
	{"POST", "/api/project/projects/*/post-cost", need("/project/projects", Approve)},
	{"GET", "/api/project/tasks", viewAny("/project/tasks", "/project/timesheets")},
	{"POST", "/api/project/tasks", need("/project/tasks", Create)},
	{"PUT", "/api/project/tasks/*", need("/project/tasks", Update)},
	{"GET", "/api/project/timesheets", need("/project/timesheets", View)},
	{"POST", "/api/project/timesheets", need("/project/timesheets", Create)},
	{"POST", "/api/project/timesheets/*/approve", need("/project/timesheets", Approve)},
	{"POST", "/api/project/timesheets/*/reject", need("/project/timesheets", Approve)},
}

// index mempercepat pencarian: method -> jumlah segmen -> rule. Path dengan
// jumlah segmen berbeda tidak mungkin cocok, jadi sebagian besar tabel tidak
// perlu disentuh sama sekali per request.
var index = buildIndex()

func buildIndex() map[string]map[int][]Rule {
	idx := map[string]map[int][]Rule{}
	for _, r := range Rules {
		byLen, ok := idx[r.Method]
		if !ok {
			byLen = map[int][]Rule{}
			idx[r.Method] = byLen
		}
		n := len(segments(r.Pattern))
		byLen[n] = append(byLen[n], r)
	}
	return idx
}

func segments(path string) []string {
	return strings.Split(strings.Trim(path, "/"), "/")
}

// Lookup mencari rule untuk sebuah request. Rule dengan segmen literal terbanyak
// menang atas rule ber-wildcard, sehingga mis. GET /api/dw/sync/status tidak
// pernah tertangkap pola GET /api/dw/analytics/*.
func Lookup(method, path string) (Rule, bool) {
	segs := segments(path)
	best := Rule{}
	bestScore := -1
	for _, r := range index[method][len(segs)] {
		score, ok := matchScore(segments(r.Pattern), segs)
		if !ok {
			continue
		}
		if score > bestScore {
			best, bestScore = r, score
		}
	}
	return best, bestScore >= 0
}

// matchScore mengembalikan jumlah segmen literal yang cocok; makin tinggi makin
// spesifik.
func matchScore(pattern, path []string) (int, bool) {
	if len(pattern) != len(path) {
		return 0, false
	}
	score := 0
	for i := range pattern {
		if pattern[i] == "*" {
			continue
		}
		if pattern[i] != path[i] {
			return 0, false
		}
		score++
	}
	return score, true
}

// Validate dipanggil dari test: menolak tabel yang punya dua rule identik
// (method + pattern), yang berarti salah satunya diam-diam tidak pernah terpakai
// dan keputusannya hanya ada di atas kertas.
func Validate() error {
	seen := map[string]bool{}
	for _, r := range Rules {
		key := r.Method + " " + r.Pattern
		if seen[key] {
			return fmt.Errorf("rule ganda: %s", key)
		}
		seen[key] = true
		if r.Req.kind == kindPermission && len(r.Req.needs) == 0 {
			return fmt.Errorf("rule %s butuh permission tapi tidak menyebut menu apa pun", key)
		}
	}
	return nil
}
