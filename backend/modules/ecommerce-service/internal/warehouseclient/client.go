// Package warehouseclient adalah panggilan HTTP langsung (service-to-service,
// tidak lewat api-gateway) ke warehouse-service untuk mencatat stok keluar
// saat order SHIPPED, mengikuti pola yang sama seperti
// sales-service/internal/warehouseclient (stok keluar saat SO FULFILLED) dan
// purchasing-service/internal/warehouseclient (stok masuk saat PO RECEIVED).
// warehouse-service tidak memvalidasi JWT (hanya gateway yang
// melakukannya), sehingga header X-User-Id diteruskan manual supaya mutasi
// stok tercatat dengan actor yang benar.
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

// MovementLineInput beda dari sales-service/purchasing-service: ecommerce-
// service selalu punya product_id nyata (order_items menunjuk langsung ke
// produk warehouse-service, tidak seperti sales_order_lines/purchase_order_
// lines yang cuma punya product_name teks bebas), jadi ProductID diisi di
// sini -- warehouse-service's postStockMovementBatch menerima product_id
// ATAU product_name per baris (lihat internal/httpapi/stock_movements.go).
type MovementLineInput struct {
	ProductID string  `json:"product_id"`
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

// PostMovementBatch mencatat sekumpulan mutasi stok (satu per baris order)
// dalam satu panggilan. Kegagalan dikembalikan sebagai error ke pemanggil
// supaya keputusan "lanjut atau tidak" (mis. tetap ubah status order lokal
// atau tidak) tetap di tangan pemanggil, konsisten dengan pola sales-service.
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
