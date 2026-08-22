package model

import "time"

type Employee struct {
	ID               string     `json:"id" db:"id"`
	CompanyID        string     `json:"company_id" db:"company_id"`
	BranchID         *string    `json:"branch_id" db:"branch_id"`
	EmployeeCode     string     `json:"employee_code" db:"employee_code"`
	FirstName        string     `json:"first_name" db:"first_name"`
	LastName         string     `json:"last_name" db:"last_name"`
	Email            string     `json:"email" db:"email"`
	Phone            string     `json:"phone" db:"phone"`
	Department       string     `json:"department" db:"department"`
	JobTitle         string     `json:"job_title" db:"job_title"`
	ManagerID        *string    `json:"manager_id" db:"manager_id"`
	EmploymentType   string     `json:"employment_type" db:"employment_type"` // PERMANENT | CONTRACT | INTERN | OUTSOURCE
	Status           string     `json:"status" db:"status"`                   // ACTIVE | INACTIVE | TERMINATED | ON_LEAVE
	HireDate         time.Time  `json:"hire_date" db:"hire_date"`
	TerminationDate  *time.Time `json:"termination_date" db:"termination_date"`
	BasicSalary      float64    `json:"basic_salary" db:"basic_salary"`
	MonthlyAllowance float64    `json:"monthly_allowance" db:"monthly_allowance"`
	PTKPStatus       string     `json:"ptkp_status" db:"ptkp_status"`
	IsActive         bool       `json:"is_active" db:"is_active"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

type AttendanceLog struct {
	ID         string     `json:"id" db:"id"`
	CompanyID  string     `json:"company_id" db:"company_id"`
	BranchID   *string    `json:"branch_id" db:"branch_id"`
	EmployeeID string     `json:"employee_id" db:"employee_id"`
	LogDate    time.Time  `json:"log_date" db:"log_date"`
	CheckIn    *time.Time `json:"check_in" db:"check_in"`
	CheckOut   *time.Time `json:"check_out" db:"check_out"`
	Source     string     `json:"source" db:"source"`
	Status     string     `json:"status" db:"status"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
}

// LeaveRequest adalah pengajuan cuti untuk satu rentang tanggal. Hanya yang
// berstatus APPROVED yang berpengaruh ke payroll (lihat internal/httpapi/payroll.go).
type LeaveRequest struct {
	ID              string     `json:"id" db:"id"`
	CompanyID       string     `json:"company_id" db:"company_id"`
	BranchID        *string    `json:"branch_id" db:"branch_id"`
	EmployeeID      string     `json:"employee_id" db:"employee_id"`
	EmployeeName    string     `json:"employee_name" db:"employee_name"`
	LeaveType       string     `json:"leave_type" db:"leave_type"` // ANNUAL | SICK | MATERNITY | UNPAID | OTHER
	StartDate       time.Time  `json:"start_date" db:"start_date"`
	EndDate         time.Time  `json:"end_date" db:"end_date"`
	TotalDays       int16      `json:"total_days" db:"total_days"`
	Reason          string     `json:"reason" db:"reason"`
	Status          string     `json:"status" db:"status"` // DRAFT | SUBMITTED | APPROVED | REJECTED | CANCELLED
	RejectionReason string     `json:"rejection_reason" db:"rejection_reason"`
	SubmittedAt     *time.Time `json:"submitted_at" db:"submitted_at"`
	DecidedAt       *time.Time `json:"decided_at" db:"decided_at"`
	DecidedBy       *string    `json:"decided_by" db:"decided_by"`
	CreatedBy       *string    `json:"created_by" db:"created_by"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// Holiday adalah satu tanggal libur di kalender sebuah company: tanggal merah
// nasional maupun libur khusus perusahaan. Keduanya sama-sama berarti "bukan
// hari kerja"; IsNational hanya membedakan asal-usulnya untuk tampilan.
type Holiday struct {
	ID          string    `json:"id" db:"id"`
	CompanyID   string    `json:"company_id" db:"company_id"`
	HolidayDate time.Time `json:"holiday_date" db:"holiday_date"`
	Name        string    `json:"name" db:"name"`
	IsNational  bool      `json:"is_national" db:"is_national"`
	CreatedBy   *string   `json:"created_by" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// LeaveQuota adalah jatah cuti tahunan seorang karyawan pada satu tahun.
// Hari yang sudah terpakai TIDAK disimpan di sini -- selalu dihitung ulang dari
// leave_requests yang APPROVED, supaya tidak ada dua sumber kebenaran.
type LeaveQuota struct {
	ID          string    `json:"id" db:"id"`
	EmployeeID  string    `json:"employee_id" db:"employee_id"`
	Year        int       `json:"year" db:"year"`
	TotalDays   int       `json:"total_days" db:"total_days"`
	CarriedOver int       `json:"carried_over" db:"carried_over"`
	Note        string    `json:"note" db:"note"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// OvertimeLog adalah catatan lembur satu karyawan pada satu tanggal. Sama
// seperti cuti, hanya yang APPROVED yang ikut ke payroll.
type OvertimeLog struct {
	ID              string     `json:"id" db:"id"`
	CompanyID       string     `json:"company_id" db:"company_id"`
	BranchID        *string    `json:"branch_id" db:"branch_id"`
	EmployeeID      string     `json:"employee_id" db:"employee_id"`
	EmployeeName    string     `json:"employee_name" db:"employee_name"`
	WorkDate        time.Time  `json:"work_date" db:"work_date"`
	Hours           float64    `json:"hours" db:"hours"`
	IsHoliday       bool       `json:"is_holiday" db:"is_holiday"`
	HourlyRate      float64    `json:"hourly_rate" db:"hourly_rate"`
	Amount          float64    `json:"amount" db:"amount"`
	Description     string     `json:"description" db:"description"`
	Status          string     `json:"status" db:"status"` // DRAFT | APPROVED | REJECTED
	RejectionReason string     `json:"rejection_reason" db:"rejection_reason"`
	DecidedAt       *time.Time `json:"decided_at" db:"decided_at"`
	DecidedBy       *string    `json:"decided_by" db:"decided_by"`
	PayrollRunID    *string    `json:"payroll_run_id" db:"payroll_run_id"`
	CreatedBy       *string    `json:"created_by" db:"created_by"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type PayrollRun struct {
	ID             string     `json:"id" db:"id"`
	CompanyID      string     `json:"company_id" db:"company_id"`
	BranchID       *string    `json:"branch_id" db:"branch_id"`
	Period         string     `json:"period" db:"period"`
	Status         string     `json:"status" db:"status"` // DRAFT | POSTED
	TotalEmployees int        `json:"total_employees" db:"total_employees"`
	TotalGross     float64    `json:"total_gross" db:"total_gross"`
	TotalPPh21     float64    `json:"total_pph21" db:"total_pph21"`
	TotalBPJS      float64    `json:"total_bpjs" db:"total_bpjs"`
	TotalDeduction float64    `json:"total_deduction" db:"total_deduction"`
	TotalOvertime  float64    `json:"total_overtime" db:"total_overtime"`
	TotalNet       float64    `json:"total_net" db:"total_net"`
	JournalID      *string    `json:"journal_id" db:"journal_id"`
	PostedBy       *string    `json:"posted_by" db:"posted_by"`
	PostedAt       *time.Time `json:"posted_at" db:"posted_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}

type PayrollDetail struct {
	ID               string    `json:"id" db:"id"`
	PayrollRunID     string    `json:"payroll_run_id" db:"payroll_run_id"`
	EmployeeID       string    `json:"employee_id" db:"employee_id"`
	EmployeeName     string    `json:"employee_name" db:"employee_name"`
	BasicSalary      float64   `json:"basic_salary" db:"basic_salary"`
	TotalAllowance   float64   `json:"total_allowance" db:"total_allowance"`
	GrossSalary      float64   `json:"gross_salary" db:"gross_salary"`
	PPh21            float64   `json:"pph21" db:"pph21"`
	BPJSKesehatanEmp float64   `json:"bpjs_kesehatan_emp" db:"bpjs_kesehatan_emp"`
	BPJSTKJHTEmp     float64   `json:"bpjs_tk_jht_emp" db:"bpjs_tk_jht_emp"`
	BPJSTKJPEmp      float64   `json:"bpjs_tk_jp_emp" db:"bpjs_tk_jp_emp"`
	TotalDeduction   float64   `json:"total_deduction" db:"total_deduction"`
	NetSalary        float64   `json:"net_salary" db:"net_salary"`
	WorkingDays      int16     `json:"working_days" db:"working_days"`
	PresentDays      int16     `json:"present_days" db:"present_days"`
	OvertimeHours    float64   `json:"overtime_hours" db:"overtime_hours"`
	OvertimePay      float64   `json:"overtime_pay" db:"overtime_pay"`
	PaidLeaveDays    int16     `json:"paid_leave_days" db:"paid_leave_days"`
	UnpaidLeaveDays  int16     `json:"unpaid_leave_days" db:"unpaid_leave_days"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// KPIIndicator adalah master indikator penilaian kinerja milik satu company.
// Bobot (Weight) dinyatakan dalam persen; jumlah bobot pada sebuah penilaian
// harus tepat 100% saat penilaian itu diajukan.
type KPIIndicator struct {
	ID          string    `json:"id" db:"id"`
	CompanyID   string    `json:"company_id" db:"company_id"`
	Code        string    `json:"code" db:"code"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Unit        string    `json:"unit" db:"unit"`
	TargetValue float64   `json:"target_value" db:"target_value"`
	Weight      float64   `json:"weight" db:"weight"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedBy   *string   `json:"created_by" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// KPIReview adalah penilaian kinerja satu karyawan pada satu periode.
// TotalScore & Rating DISIMPAN, bukan dihitung saat dibaca, supaya hasil yang
// sudah disetujui tidak berubah kalau rumusnya nanti disesuaikan.
type KPIReview struct {
	ID              string     `json:"id" db:"id"`
	CompanyID       string     `json:"company_id" db:"company_id"`
	BranchID        *string    `json:"branch_id" db:"branch_id"`
	EmployeeID      string     `json:"employee_id" db:"employee_id"`
	EmployeeName    string     `json:"employee_name" db:"employee_name"`
	Period          string     `json:"period" db:"period"`
	Status          string     `json:"status" db:"status"` // DRAFT | SUBMITTED | APPROVED | REJECTED
	TotalScore      float64    `json:"total_score" db:"total_score"`
	Rating          string     `json:"rating" db:"rating"`
	Notes           string     `json:"notes" db:"notes"`
	RejectionReason string     `json:"rejection_reason" db:"rejection_reason"`
	SubmittedAt     *time.Time `json:"submitted_at" db:"submitted_at"`
	DecidedAt       *time.Time `json:"decided_at" db:"decided_at"`
	DecidedBy       *string    `json:"decided_by" db:"decided_by"`
	CreatedBy       *string    `json:"created_by" db:"created_by"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// KPIReviewItem adalah satu indikator di dalam sebuah penilaian. Nama, unit,
// target, dan bobot adalah SALINAN saat penilaian dibuat -- mengubah master
// indikator tidak boleh menulis ulang hasil periode yang sudah lewat.
type KPIReviewItem struct {
	ID            string  `json:"id" db:"id"`
	ReviewID      string  `json:"review_id" db:"review_id"`
	IndicatorID   string  `json:"indicator_id" db:"indicator_id"`
	IndicatorName string  `json:"indicator_name" db:"indicator_name"`
	Unit          string  `json:"unit" db:"unit"`
	TargetValue   float64 `json:"target_value" db:"target_value"`
	Weight        float64 `json:"weight" db:"weight"`
	ActualValue   float64 `json:"actual_value" db:"actual_value"`
	Achievement   float64 `json:"achievement" db:"achievement"`
	Score         float64 `json:"score" db:"score"`
	Note          string  `json:"note" db:"note"`
}
