// Package financeclient adalah panggilan HTTP langsung (service-to-service,
// tidak lewat api-gateway) ke finance-service untuk membuat & memposting
// invoice AP dari purchase order, mengikuti pola yang sama seperti
// sales-service/internal/financeclient (yang membuat invoice AR). finance-service
// tidak memvalidasi JWT (hanya gateway yang melakukannya), sehingga header
// X-User-Id diteruskan manual supaya invoice tercatat dengan actor yang benar.
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

type InvoiceLineInput struct {
	AccountID   string  `json:"account_id"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type CreateInvoiceRequest struct {
	CompanyID        string             `json:"company_id"`
	BranchID         *string            `json:"branch_id,omitempty"`
	InvoiceType      string             `json:"invoice_type"`
	PartnerName      string             `json:"partner_name"`
	InvoiceDate      string             `json:"invoice_date"`
	ControlAccountID string             `json:"control_account_id"`
	TaxAccountID     *string            `json:"tax_account_id,omitempty"`
	TaxAmount        float64            `json:"tax_amount"`
	Lines            []InvoiceLineInput `json:"lines"`
}

type InvoiceResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// CreateAndPostInvoice membuat invoice AP (status DRAFT) lalu langsung
// mem-posting-nya (DRAFT -> POSTED), meniru satu langkah "buat invoice" dari
// sisi pemanggil (Purchasing) walau finance-service sendiri membagi dua
// endpoint terpisah.
func (c *Client) CreateAndPostInvoice(actorUserID string, req CreateInvoiceRequest) (*InvoiceResult, error) {
	created, err := c.postJSON("/invoices", actorUserID, req)
	if err != nil {
		return nil, fmt.Errorf("create invoice: %w", err)
	}
	var inv InvoiceResult
	if err := json.Unmarshal(created, &inv); err != nil {
		return nil, fmt.Errorf("decode invoice: %w", err)
	}

	if _, err := c.postJSON("/invoices/"+inv.ID+"/post", actorUserID, nil); err != nil {
		return nil, fmt.Errorf("post invoice: %w", err)
	}
	inv.Status = "POSTED"
	return &inv, nil
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
