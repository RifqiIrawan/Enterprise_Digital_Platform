// Package hrclient adalah panggilan HTTP langsung (service-to-service, tidak
// lewat api-gateway) ke hr-service, mengikuti pola yang sama seperti
// fleet-service/internal/ecommerceclient dan
// ecommerce-service/internal/warehouseclient. hr-service tidak memvalidasi JWT
// (hanya gateway yang melakukannya).
//
// Arah panggilan SENGAJA project -> hr, tidak pernah sebaliknya: hr-service
// tidak boleh tahu apa pun soal proyek. Konsekuensinya hr-service yang mati
// hanya memblokir PENUGASAN orang (bikin/ubah proyek-tugas-timesheet dengan
// employee_id), bukan seluruh modul -- tugas backlog tanpa assignee dan
// perubahan status tetap jalan.
package hrclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
}

// Employee adalah subset field hr-service yang benar-benar dipakai
// project-service: identitas untuk di-snapshot, company_id untuk guard
// lintas-company, status untuk menolak penugasan ke karyawan non-aktif, dan
// basic_salary untuk menurunkan tarif per jam default. Field lain di response
// sengaja diabaikan supaya perubahan bentuk response hr-service yang tidak
// menyentuh field-field ini tidak memecahkan modul ini.
//
// hr-service TIDAK punya field "name" tunggal -- namanya terpisah first_name
// dan last_name (dikonfirmasi dengan membaca model.Employee di hr-service,
// bukan ditebak), jadi penggabungannya dilakukan di sini lewat FullName.
type Employee struct {
	ID          string  `json:"id"`
	CompanyID   string  `json:"company_id"`
	FirstName   string  `json:"first_name"`
	LastName    string  `json:"last_name"`
	Status      string  `json:"status"`
	BasicSalary float64 `json:"basic_salary"`
}

func (e Employee) FullName() string {
	return strings.TrimSpace(e.FirstName + " " + e.LastName)
}

// GetEmployee mengambil satu karyawan untuk divalidasi + di-snapshot.
// GET /employees/{id} di hr-service mengembalikan model.Employee polos (bentuk
// JSON FLAT, tidak dibungkus key "employee") -- dikonfirmasi dengan membaca
// handler getEmployee-nya langsung.
func (c *Client) GetEmployee(employeeID string) (*Employee, error) {
	body, err := c.do(http.MethodGet, "/employees/"+employeeID)
	if err != nil {
		return nil, fmt.Errorf("get employee: %w", err)
	}
	var e Employee
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("decode employee: %w", err)
	}
	if e.ID == "" {
		return nil, fmt.Errorf("hr-service returned an employee without id")
	}
	return &e, nil
}

func (c *Client) do(method, path string) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hr-service returned %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
