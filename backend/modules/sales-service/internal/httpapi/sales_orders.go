package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/sales-service/internal/financeclient"
	"github.com/enterprise-digital-platform/sales-service/internal/model"
)

const salesOrderColumns = `id, company_id, branch_id, so_number, customer_id, quotation_id, order_date, status, subtotal_amount, tax_amount, total_amount, invoice_id, created_at, updated_at`

func scanSalesOrder(row pgx.Row, so *model.SalesOrder) error {
	return row.Scan(&so.ID, &so.CompanyID, &so.BranchID, &so.SONumber, &so.CustomerID, &so.QuotationID, &so.OrderDate,
		&so.Status, &so.SubtotalAmount, &so.TaxAmount, &so.TotalAmount, &so.InvoiceID, &so.CreatedAt, &so.UpdatedAt)
}

func (h *Handler) listSalesOrders(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT `+salesOrderColumns+` FROM sales_orders WHERE company_id = $1 ORDER BY order_date DESC, so_number DESC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data sales order")
		return
	}
	defer rows.Close()

	orders := []model.SalesOrder{}
	for rows.Next() {
		var so model.SalesOrder
		if err := scanSalesOrder(rows, &so); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data sales order")
			return
		}
		orders = append(orders, so)
	}
	writeJSON(w, http.StatusOK, orders)
}

type createSalesOrderRequest struct {
	CompanyID  string      `json:"company_id"`
	BranchID   *string     `json:"branch_id"`
	CustomerID string      `json:"customer_id"`
	OrderDate  string      `json:"order_date"`
	Lines      []lineInput `json:"lines"`
}

type salesOrderWithLines struct {
	model.SalesOrder
	Lines []model.SalesOrderLine `json:"lines"`
}

func (h *Handler) createSalesOrder(w http.ResponseWriter, r *http.Request) {
	var req createSalesOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.CustomerID == "" || req.OrderDate == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, customer_id, order_date, dan minimal 1 baris wajib diisi")
		return
	}
	orderDate, err := time.Parse("2006-01-02", req.OrderDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "order_date harus format YYYY-MM-DD")
		return
	}

	var subtotal float64
	for _, l := range req.Lines {
		if strings.TrimSpace(l.ProductName) == "" {
			writeError(w, http.StatusBadRequest, "Setiap baris wajib punya product_name")
			return
		}
		subtotal += l.Quantity * l.UnitPrice
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	period := req.OrderDate[:7]
	soNumber, err := nextSequence(ctx, tx, req.CompanyID, "sales_orders", "so_number", "SO", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor sales order")
		return
	}

	var so model.SalesOrder
	err = tx.QueryRow(ctx, `
		INSERT INTO sales_orders (company_id, branch_id, so_number, customer_id, order_date, subtotal_amount, tax_amount, total_amount)
		VALUES ($1, $2, $3, $4, $5, $6, 0, $6)
		RETURNING `+salesOrderColumns,
		req.CompanyID, req.BranchID, soNumber, req.CustomerID, orderDate, subtotal,
	).Scan(&so.ID, &so.CompanyID, &so.BranchID, &so.SONumber, &so.CustomerID, &so.QuotationID, &so.OrderDate,
		&so.Status, &so.SubtotalAmount, &so.TaxAmount, &so.TotalAmount, &so.InvoiceID, &so.CreatedAt, &so.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat sales order")
		return
	}

	lines := make([]model.SalesOrderLine, 0, len(req.Lines))
	for i, l := range req.Lines {
		amount := l.Quantity * l.UnitPrice
		var line model.SalesOrderLine
		err := tx.QueryRow(ctx, `
			INSERT INTO sales_order_lines (sales_order_id, line_number, product_name, description, quantity, unit_price, amount)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, sales_order_id, line_number, product_name, description, quantity, unit_price, amount`,
			so.ID, i+1, l.ProductName, l.Description, l.Quantity, l.UnitPrice, amount,
		).Scan(&line.ID, &line.SalesOrderID, &line.LineNumber, &line.ProductName, &line.Description, &line.Quantity, &line.UnitPrice, &line.Amount)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris sales order")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan sales order")
		return
	}

	h.events.Publish("sales.order.created", newAuditEvent("sales.order.created", actorFromHeader(r), &so.CompanyID, "create", "sales_order", so.ID, so))
	writeJSON(w, http.StatusCreated, salesOrderWithLines{SalesOrder: so, Lines: lines})
}

func (h *Handler) getSalesOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var so model.SalesOrder
	err := scanSalesOrder(h.pool.QueryRow(ctx, `SELECT `+salesOrderColumns+` FROM sales_orders WHERE id = $1`, id), &so)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Sales order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat sales order")
		return
	}

	lines, err := h.fetchSalesOrderLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris sales order")
		return
	}
	writeJSON(w, http.StatusOK, salesOrderWithLines{SalesOrder: so, Lines: lines})
}

func (h *Handler) fetchSalesOrderLines(ctx context.Context, salesOrderID string) ([]model.SalesOrderLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, sales_order_id, line_number, product_name, description, quantity, unit_price, amount
		FROM sales_order_lines WHERE sales_order_id = $1 ORDER BY line_number ASC`, salesOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.SalesOrderLine{}
	for rows.Next() {
		var l model.SalesOrderLine
		if err := rows.Scan(&l.ID, &l.SalesOrderID, &l.LineNumber, &l.ProductName, &l.Description, &l.Quantity, &l.UnitPrice, &l.Amount); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (h *Handler) transitionSalesOrder(w http.ResponseWriter, r *http.Request, from, to, eventType string) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var so model.SalesOrder
	err := scanSalesOrder(h.pool.QueryRow(r.Context(), `
		UPDATE sales_orders SET status = $1, updated_at = now() WHERE id = $2 AND status = $3
		RETURNING `+salesOrderColumns, to, id, from), &so)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, fmt.Sprintf("Sales order tidak ditemukan atau tidak berstatus %s", from))
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status sales order")
		return
	}

	h.events.Publish(eventType, newAuditEvent(eventType, actor, &so.CompanyID, "update", "sales_order", so.ID, so))
	writeJSON(w, http.StatusOK, so)
}

func (h *Handler) confirmSalesOrder(w http.ResponseWriter, r *http.Request) {
	h.transitionSalesOrder(w, r, "DRAFT", "CONFIRMED", "sales.order.confirmed")
}

func (h *Handler) fulfillSalesOrder(w http.ResponseWriter, r *http.Request) {
	h.transitionSalesOrder(w, r, "CONFIRMED", "FULFILLED", "sales.order.fulfilled")
}

type invoiceSalesOrderRequest struct {
	RevenueAccountID string `json:"revenue_account_id"`
	ControlAccountID string `json:"control_account_id"`
	TaxAccountID     string `json:"tax_account_id"`
}

// invoiceSalesOrder membuat invoice AR di finance-service lewat panggilan
// HTTP langsung (lihat internal/financeclient), lalu menyimpan invoice_id di
// sales_orders dan menandai status INVOICED. Karena ini dua database
// terpisah tanpa distributed transaction, panggilan finance-service dulu,
// baru update status lokal setelah sukses -- konsisten dengan pola
// hr-service saat posting payroll ke GL.
func (h *Handler) invoiceSalesOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req invoiceSalesOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.RevenueAccountID == "" || req.ControlAccountID == "" {
		writeError(w, http.StatusBadRequest, "revenue_account_id dan control_account_id wajib diisi")
		return
	}

	var so model.SalesOrder
	err := scanSalesOrder(h.pool.QueryRow(ctx, `SELECT `+salesOrderColumns+` FROM sales_orders WHERE id = $1`, id), &so)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Sales order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat sales order")
		return
	}
	if so.Status != "CONFIRMED" && so.Status != "FULFILLED" {
		writeError(w, http.StatusConflict, "Sales order harus berstatus CONFIRMED atau FULFILLED sebelum di-invoice")
		return
	}
	if so.InvoiceID != nil {
		writeError(w, http.StatusConflict, "Sales order ini sudah pernah di-invoice")
		return
	}

	var customerName string
	if err := h.pool.QueryRow(ctx, `SELECT name FROM customers WHERE id = $1`, so.CustomerID).Scan(&customerName); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data customer")
		return
	}

	lines, err := h.fetchSalesOrderLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris sales order")
		return
	}

	financeLines := make([]financeclient.InvoiceLineInput, 0, len(lines))
	for _, l := range lines {
		financeLines = append(financeLines, financeclient.InvoiceLineInput{
			AccountID:   req.RevenueAccountID,
			Description: l.ProductName,
			Quantity:    l.Quantity,
			UnitPrice:   l.UnitPrice,
		})
	}

	var taxAccountID *string
	if req.TaxAccountID != "" {
		taxAccountID = &req.TaxAccountID
	}

	inv, err := h.finance.CreateAndPostInvoice(headerValue(actor), financeclient.CreateInvoiceRequest{
		CompanyID:        so.CompanyID,
		BranchID:         so.BranchID,
		InvoiceType:      "AR",
		PartnerName:      customerName,
		InvoiceDate:      so.OrderDate.Format("2006-01-02"),
		ControlAccountID: req.ControlAccountID,
		TaxAccountID:     taxAccountID,
		TaxAmount:        so.TaxAmount,
		Lines:            financeLines,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal membuat invoice di finance-service: %v", err))
		return
	}

	err = scanSalesOrder(h.pool.QueryRow(ctx, `
		UPDATE sales_orders SET status = 'INVOICED', invoice_id = $1, updated_at = now()
		WHERE id = $2
		RETURNING `+salesOrderColumns, inv.ID, id), &so)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Invoice berhasil dibuat di finance-service, tetapi gagal memperbarui status sales order lokal")
		return
	}

	h.events.Publish("sales.order.invoiced", newAuditEvent("sales.order.invoiced", actor, &so.CompanyID, "invoice", "sales_order", so.ID, so))
	writeJSON(w, http.StatusOK, so)
}
