// Package warehouseclient adalah panggilan HTTP langsung (service-to-service,
// tidak lewat api-gateway) ke warehouse-service untuk mencatat konsumsi bahan
// baku (stock OUT) dan penambahan produk jadi (stock IN) saat work order
// COMPLETED, mengikuti pola yang sama seperti purchasing-service/sales-service
// internal/warehouseclient. Beda dengan PO/SO yang hanya punya product_name
// teks bebas, production-service sudah tahu product_id pasti (dari BOM yang
// merujuk ke warehouse-service products), jadi baris di sini dikirim pakai
// product_id supaya tidak perlu name-matching di warehouse-service.
package warehouseclient

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

type MovementLineInput struct {
	ProductID string  `json:"product_id,omitempty"`
	Quantity  float64 `json:"quantity"`
}

type PostMovementBatchRequest struct {
	CompanyID     string              `json:"company_id"`
	BranchID      *string             `json:"branch_id,omitempty"`
	WarehouseID   string              `json:"warehouse_id"`
	MovementType  string              `json:"movement_type"`
	ReferenceType string              `json:"reference_type"`
	ReferenceID   string              `json:"reference_id"`
	Notes         string              `json:"notes"`
	MovementDate  string              `json:"movement_date,omitempty"`
	Lines         []MovementLineInput `json:"lines"`
}

// PostMovementBatch mencatat sekumpulan mutasi stok dalam satu panggilan.
// Kegagalan dikembalikan sebagai error ke pemanggil supaya keputusan "lanjut
// atau tidak" tetap di tangan pemanggil, konsisten dengan pola financeclient.
func (c *Client) PostMovementBatch(actorUserID string, req PostMovementBatchRequest) error {
	_, err := c.postJSON("/stock-movements/batch", actorUserID, req)
	if err != nil {
		return fmt.Errorf("post stock movement batch: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("warehouse-service returned %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
