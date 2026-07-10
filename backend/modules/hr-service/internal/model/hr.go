package model

import "time"

type Employee struct {
	ID               string    `json:"id" db:"id"`
	CompanyID        string    `json:"company_id" db:"company_id"`
	BranchID         *string   `json:"branch_id" db:"branch_id"`
	EmployeeCode     string    `json:"employee_code" db:"employee_code"`
	FirstName        string    `json:"first_name" db:"first_name"`
	LastName         string    `json:"last_name" db:"last_name"`
	Email            string    `json:"email" db:"email"`
	Phone            string    `json:"phone" db:"phone"`
	Department       string    `json:"department" db:"department"`
	JobTitle         string    `json:"job_title" db:"job_title"`
	ManagerID        *string   `json:"manager_id" db:"manager_id"`
	EmploymentType   string    `json:"employment_type" db:"employment_type"` // PERMANENT | CONTRACT | INTERN | OUTSOURCE
	Status           string    `json:"status" db:"status"`                   // ACTIVE | INACTIVE | TERMINATED | ON_LEAVE
	HireDate         time.Time `json:"hire_date" db:"hire_date"`
	TerminationDate  *time.Time `json:"termination_date" db:"termination_date"`
	BasicSalary      float64   `json:"basic_salary" db:"basic_salary"`
	MonthlyAllowance float64   `json:"monthly_allowance" db:"monthly_allowance"`
	PTKPStatus       string    `json:"ptkp_status" db:"ptkp_status"`
	IsActive         bool      `json:"is_active" db:"is_active"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type AttendanceLog struct {
	ID         string     `json:"id" db:"id"`
	CompanyID  string     `json:"company_id" db:"company_id"`
	EmployeeID string     `json:"employee_id" db:"employee_id"`
	LogDate    time.Time  `json:"log_date" db:"log_date"`
	CheckIn    *time.Time `json:"check_in" db:"check_in"`
	CheckOut   *time.Time `json:"check_out" db:"check_out"`
	Source     string     `json:"source" db:"source"`
	Status     string     `json:"status" db:"status"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
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
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}
