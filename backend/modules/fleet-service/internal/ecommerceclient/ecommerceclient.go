// Package ecommerceclient adalah panggilan HTTP langsung (service-to-service,
// tidak lewat api-gateway) ke ecommerce-service, mengikuti pola yang sama
// seperti ecommerce-service/internal/warehouseclient dan
// sales-service/internal/warehouseclient. ecommerce-service tidak memvalidasi
// JWT (hanya gateway yang melakukannya), sehingga header X-User-Id diteruskan
// manual supaya perubahan status order tercatat dengan actor yang benar.
//
// Arah panggilan SENGAJA fleet -> ecommerce, bukan sebaliknya: kalau
// ecommerce-service yang memanggil fleet-service saat order SHIPPED, jalur
// kritis checkout jadi ikut gagal setiap kali fleet-service mati. Di arah ini
// ecommerce-service tidak tahu sama sekali soal fleet-service, persis seperti
// warehouse-service tidak tahu soal ecommerce-service.
package ecommerceclient

import (
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

// Order adalah subset field ecommerce-service yang benar-benar dipakai
// fleet-service (nomor order + identitas/alamat penerima untuk di-snapshot,
// plus status untuk validasi). Field lain di response sengaja diabaikan
// supaya perubahan bentuk response ecommerce-service yang tidak menyentuh
// field-field ini tidak memecahkan modul ini.
type Order struct {
	ID              string `json:"id"`
	CompanyID       string `json:"company_id"`
	OrderNumber     string `json:"order_number"`
	CustomerName    string `json:"customer_name"`
	ShippingAddress string `json:"shipping_address"`
	Status          string `json:"status"`
}

// GetOrder mengambil satu order untuk di-snapshot ke delivery order.
// Kegagalan dikembalikan sebagai error supaya keputusan "lanjut atau tidak"
// tetap di tangan pemanggil, konsisten dengan pola warehouseclient.
func (c *Client) GetOrder(orderID string) (*Order, error) {
	body, err := c.do(http.MethodGet, "/orders/"+orderID, "", nil)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	// GET /orders/{id} mengembalikan model.OrderWithItems, yang MENG-EMBED
	// Order tanpa json tag -- jadi bentuk JSON-nya FLAT (`{"id":..., "items":
	// [...]}`), bukan bersarang di bawah key "order". Dikonfirmasi dengan
	// membaca definisi struct-nya di ecommerce-service, bukan ditebak dari
	// nama tipenya. Field "items" diabaikan: fleet-service cuma butuh header
	// order-nya.
	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("decode order: %w", err)
	}
	if order.ID == "" {
		return nil, fmt.Errorf("ecommerce-service returned an order without id")
	}
	return &order, nil
}

// MarkDelivered memajukan order e-commerce ke DELIVERED. Dipanggil saat
// delivery order fisiknya selesai -- fleet-service adalah pihak yang benar
// benar tahu barangnya sudah sampai, jadi dia yang menggerakkan status di
// ecommerce-service (arah yang sama seperti ecommerce-service menggerakkan
// stok di warehouse-service saat SHIPPED).
func (c *Client) MarkDelivered(actorUserID, orderID string) error {
	if _, err := c.do(http.MethodPost, "/orders/"+orderID+"/deliver", actorUserID, nil); err != nil {
		return fmt.Errorf("mark order delivered: %w", err)
	}
	return nil
}

func (c *Client) do(method, path, actorUserID string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
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
		return nil, fmt.Errorf("ecommerce-service returned %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
