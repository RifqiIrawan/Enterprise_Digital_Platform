package model

import "time"

// Project. ManagerEmployeeID/ManagerName nullable karena proyek boleh dibuat
// dulu sebelum manajernya ditentukan. CompletedAt/CancelledAt pointer karena
// genuinely NULL sampai transisinya terjadi -- bukan string kosong, pelajaran
// dari bug scan-NULL opportunities.lost_reason di crm-service.
type Project struct {
	ID                string     `json:"id"`
	CompanyID         string     `json:"company_id"`
	BranchID          *string    `json:"branch_id"`
	ProjectCode       string     `json:"project_code"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	CustomerName      string     `json:"customer_name"`
	ManagerEmployeeID *string    `json:"manager_employee_id"`
	ManagerName       *string    `json:"manager_name"`
	StartDate         time.Time  `json:"start_date"`
	EndDate           *time.Time `json:"end_date"`
	Status            string     `json:"status"`
	BudgetAmount      float64    `json:"budget_amount"`
	ActualCost        float64    `json:"actual_cost"`
	CompletedAt       *time.Time `json:"completed_at"`
	CancelledAt       *time.Time `json:"cancelled_at"`
	Notes             string     `json:"notes"`
	CreatedByUserID   *string    `json:"created_by_user_id"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Task. AssigneeEmployeeID/AssigneeName nullable: tugas boleh dibuat sebagai
// backlog yang belum ditugaskan ke siapa pun.
type Task struct {
	ID                 string     `json:"id"`
	CompanyID          string     `json:"company_id"`
	BranchID           *string    `json:"branch_id"`
	ProjectID          string     `json:"project_id"`
	TaskNumber         string     `json:"task_number"`
	Title              string     `json:"title"`
	Description        string     `json:"description"`
	AssigneeEmployeeID *string    `json:"assignee_employee_id"`
	AssigneeName       *string    `json:"assignee_name"`
	Status             string     `json:"status"`
	Priority           string     `json:"priority"`
	DueDate            *time.Time `json:"due_date"`
	EstimatedHours     float64    `json:"estimated_hours"`
	CompletedAt        *time.Time `json:"completed_at"`
	CreatedByUserID    *string    `json:"created_by_user_id"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Timesheet. JournalEntryID terisi hanya setelah baris ini benar-benar
// diposting ke GL finance-service, jadi dia sekaligus jadi bukti audit bahwa
// biayanya sudah masuk jurnal -- bukan sekadar penanda status lokal.
type Timesheet struct {
	ID              string     `json:"id"`
	CompanyID       string     `json:"company_id"`
	BranchID        *string    `json:"branch_id"`
	ProjectID       string     `json:"project_id"`
	TaskID          *string    `json:"task_id"`
	EmployeeID      string     `json:"employee_id"`
	EmployeeName    string     `json:"employee_name"`
	WorkDate        time.Time  `json:"work_date"`
	Hours           float64    `json:"hours"`
	HourlyRate      float64    `json:"hourly_rate"`
	Amount          float64    `json:"amount"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	ApprovedAt      *time.Time `json:"approved_at"`
	PostedAt        *time.Time `json:"posted_at"`
	JournalEntryID  *string    `json:"journal_entry_id"`
	CreatedByUserID *string    `json:"created_by_user_id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
