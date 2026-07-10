// Package financeclient adalah panggilan HTTP langsung (service-to-service,
// tidak lewat api-gateway) ke finance-service, mengikuti pola
// financeClient.postJournalEntry() yang didokumentasikan di
// 20_Implementation_Guide.md. finance-service tidak memvalidasi JWT (hanya
// gateway yang melakukannya), sehingga header X-User-Id diteruskan manual
// supaya jurnal payroll tercatat dengan actor yang benar.
package financeclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
}

type JournalLineInput struct {
	AccountID    string  `json:"account_id"`
	DebitAmount  float64 `json:"debit_amount"`
	CreditAmount float64 `json:"credit_amount"`
	Description  string  `json:"description"`
}

type CreateJournalEntryRequest struct {
	CompanyID     string             `json:"company_id"`
	BranchID      *string            `json:"branch_id,omitempty"`
	EntryDate     string             `json:"entry_date"`
	Description   string             `json:"description"`
	ReferenceType string             `json:"reference_type"`
	ReferenceID   *string            `json:"reference_id,omitempty"`
	Lines         []JournalLineInput `json:"lines"`
}

type JournalEntryResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// CreateAndPostJournalEntry membuat journal entry (status DRAFT) lalu langsung
// mem-posting-nya (DRAFT -> POSTED), meniru satu langkah "post ke GL" dari sisi
// pemanggil (HR) walau finance-service sendiri membagi dua endpoint terpisah.
func (c *Client) CreateAndPostJournalEntry(actorUserID string, req CreateJournalEntryRequest) (*JournalEntryResult, error) {
	created, err := c.postJSON("/journal-entries", actorUserID, req)
	if err != nil {
		return nil, fmt.Errorf("create journal entry: %w", err)
	}
	var entry JournalEntryResult
	if err := json.Unmarshal(created, &entry); err != nil {
		return nil, fmt.Errorf("decode journal entry: %w", err)
	}

	if _, err := c.postJSON("/journal-entries/"+entry.ID+"/post", actorUserID, nil); err != nil {
		return nil, fmt.Errorf("post journal entry: %w", err)
	}
	entry.Status = "POSTED"
	return &entry, nil
}

func (c *Client) postJSON(path, actorUserID string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if actorUserID != "" {
		req.Header.Set("X-User-Id", actorUserID)
	}

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
		return nil, fmt.Errorf("finance-service returned %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
