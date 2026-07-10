package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/purchasing-service/internal/financeclient"
	"github.com/enterprise-digital-platform/purchasing-service/internal/model"
)

const purchaseOrderColumns = `id, company_id, branch_id, po_number, supplier_id, requisition_id, order_date, status, subtotal_amount, tax_amount, total_amount, invoice_id, created_at, updated_at`

func scanPurchaseOrder(row pgx.Row, po *model.PurchaseOrder) error {
	return row.Scan(&po.ID, &po.CompanyID, &po.BranchID, &po.PONumber, &po.SupplierID, &po.RequisitionID, &po.OrderDate,
		&po.Status, &po.SubtotalAmount, &po.TaxAmount, &po.TotalAmount, &po.InvoiceID, &po.CreatedAt, &po.UpdatedAt)
}

func (h *Handler) listPurchaseOrders(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT `+purchaseOrderColumns+` FROM purchase_orders WHERE company_id = $1 ORDER BY order_date DESC, po_number DESC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data purchase order")
		return
	}
	defer rows.Close()

	orders := []model.PurchaseOrder{}
	for rows.Next() {
		var po model.PurchaseOrder
		if err := scanPurchaseOrder(rows, &po); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data purchase order")
			return
		}
		orders = append(orders, po)
	}
	writeJSON(w, http.StatusOK, orders)
}

type poLineInput struct {
	ProductName string  `json:"product_name"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type createPurchaseOrderRequest struct {
	CompanyID  string        `json:"company_id"`
	BranchID   *string       `json:"branch_id"`
	SupplierID string        `json:"supplier_id"`
	OrderDate  string        `json:"order_date"`
	Lines      []poLineInput `json:"lines"`
}

type purchaseOrderWithLines struct {
	model.PurchaseOrder
	Lines []model.PurchaseOrderLine `json:"lines"`
}

func (h *Handler) createPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	var req createPurchaseOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.SupplierID == "" || req.OrderDate == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, supplier_id, order_date, dan minimal 1 baris wajib diisi")
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
	poNumber, err := nextSequence(ctx, tx, req.CompanyID, "purchase_orders", "po_number", "PO", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor purchase order")
		return
	}

	var po model.PurchaseOrder
	err = tx.QueryRow(ctx, `
		INSERT INTO purchase_orders (company_id, branch_id, po_number, supplier_id, order_date, subtotal_amount, tax_amount, total_amount)
		VALUES ($1, $2, $3, $4, $5, $6, 0, $6)
		RETURNING `+purchaseOrderColumns,
		req.CompanyID, req.BranchID, poNumber, req.SupplierID, orderDate, subtotal,
	).Scan(&po.ID, &po.CompanyID, &po.BranchID, &po.PONumber, &po.SupplierID, &po.RequisitionID, &po.OrderDate,
		&po.Status, &po.SubtotalAmount, &po.TaxAmount, &po.TotalAmount, &po.InvoiceID, &po.CreatedAt, &po.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat purchase order")
		return
	}

	lines := make([]model.PurchaseOrderLine, 0, len(req.Lines))
	for i, l := range req.Lines {
		amount := l.Quantity * l.UnitPrice
		var line model.PurchaseOrderLine
		err := tx.QueryRow(ctx, `
			INSERT INTO purchase_order_lines (purchase_order_id, line_number, product_name, description, quantity, unit_price, amount)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, purchase_order_id, line_number, product_name, description, quantity, unit_price, amount`,
			po.ID, i+1, l.ProductName, l.Description, l.Quantity, l.UnitPrice, amount,
		).Scan(&line.ID, &line.PurchaseOrderID, &line.LineNumber, &line.ProductName, &line.Description, &line.Quantity, &line.UnitPrice, &line.Amount)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris purchase order")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan purchase order")
		return
	}

	h.events.Publish("purchasing.order.created", newAuditEvent("purchasing.order.created", actorFromHeader(r), &po.CompanyID, "create", "purchase_order", po.ID, po))
	writeJSON(w, http.StatusCreated, purchaseOrderWithLines{PurchaseOrder: po, Lines: lines})
}

func (h *Handler) getPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var po model.PurchaseOrder
	err := scanPurchaseOrder(h.pool.QueryRow(ctx, `SELECT `+purchaseOrderColumns+` FROM purchase_orders WHERE id = $1`, id), &po)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Purchase order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat purchase order")
		return
	}

	lines, err := h.fetchPurchaseOrderLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris purchase order")
		return
	}
	writeJSON(w, http.StatusOK, purchaseOrderWithLines{PurchaseOrder: po, Lines: lines})
}

func (h *Handler) fetchPurchaseOrderLines(ctx context.Context, purchaseOrderID string) ([]model.PurchaseOrderLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, purchase_order_id, line_number, product_name, description, quantity, unit_price, amount
		FROM purchase_order_lines WHERE purchase_order_id = $1 ORDER BY line_number ASC`, purchaseOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.PurchaseOrderLine{}
	for rows.Next() {
		var l model.PurchaseOrderLine
		if err := rows.Scan(&l.ID, &l.PurchaseOrderID, &l.LineNumber, &l.ProductName, &l.Description, &l.Quantity, &l.UnitPrice, &l.Amount); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (h *Handler) transitionPurchaseOrder(w http.ResponseWriter, r *http.Request, from, to, eventType string) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var po model.PurchaseOrder
	err := scanPurchaseOrder(h.pool.QueryRow(r.Context(), `
		UPDATE purchase_orders SET status = $1, updated_at = now() WHERE id = $2 AND status = $3
		RETURNING `+purchaseOrderColumns, to, id, from), &po)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, fmt.Sprintf("Purchase order tidak ditemukan atau tidak berstatus %s", from))
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status purchase order")
		return
	}

	h.events.Publish(eventType, newAuditEvent(eventType, actor, &po.CompanyID, "update", "purchase_order", po.ID, po))
	writeJSON(w, http.StatusOK, po)
}

func (h *Handler) confirmPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	h.transitionPurchaseOrder(w, r, "DRAFT", "CONFIRMED", "purchasing.order.confirmed")
}

func (h *Handler) receivePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	h.transitionPurchaseOrder(w, r, "CONFIRMED", "RECEIVED", "purchasing.order.received")
}

type invoicePurchaseOrderRequest struct {
	ExpenseAccountID string `json:"expense_account_id"`
	ControlAccountID string `json:"control_account_id"`
	TaxAccountID     string `json:"tax_account_id"`
}

// invoicePurchaseOrder membuat invoice AP di finance-service lewat panggilan
// HTTP langsung (lihat internal/financeclient), lalu menyimpan invoice_id di
// purchase_orders dan menandai status INVOICED. Karena ini dua database
// terpisah tanpa distributed transaction, panggilan finance-service dulu,
// baru update status lokal setelah sukses -- konsisten dengan pola
// sales-service saat membuat invoice AR.
func (h *Handler) invoicePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req invoicePurchaseOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.ExpenseAccountID == "" || req.ControlAccountID == "" {
		writeError(w, http.StatusBadRequest, "expense_account_id dan control_account_id wajib diisi")
		return
	}

	var po model.PurchaseOrder
	err := scanPurchaseOrder(h.pool.QueryRow(ctx, `SELECT `+purchaseOrderColumns+` FROM purchase_orders WHERE id = $1`, id), &po)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Purchase order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat purchase order")
		return
	}
	if po.Status != "CONFIRMED" && po.Status != "RECEIVED" {
		writeError(w, http.StatusConflict, "Purchase order harus berstatus CONFIRMED atau RECEIVED sebelum di-invoice")
		return
	}
	if po.InvoiceID != nil {
		writeError(w, http.StatusConflict, "Purchase order ini sudah pernah di-invoice")
		return
	}

	var supplierName string
	if err := h.pool.QueryRow(ctx, `SELECT name FROM suppliers WHERE id = $1`, po.SupplierID).Scan(&supplierName); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data supplier")
		return
	}

	lines, err := h.fetchPurchaseOrderLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris purchase order")
		return
	}

	financeLines := make([]financeclient.InvoiceLineInput, 0, len(lines))
	for _, l := range lines {
		financeLines = append(financeLines, financeclient.InvoiceLineInput{
			AccountID:   req.ExpenseAccountID,
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
		CompanyID:        po.CompanyID,
		BranchID:         po.BranchID,
		InvoiceType:      "AP",
		PartnerName:      supplierName,
		InvoiceDate:      po.OrderDate.Format("2006-01-02"),
		ControlAccountID: req.ControlAccountID,
		TaxAccountID:     taxAccountID,
		TaxAmount:        po.TaxAmount,
		Lines:            financeLines,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal membuat invoice di finance-service: %v", err))
		return
	}

	err = scanPurchaseOrder(h.pool.QueryRow(ctx, `
		UPDATE purchase_orders SET status = 'INVOICED', invoice_id = $1, updated_at = now()
		WHERE id = $2
		RETURNING `+purchaseOrderColumns, inv.ID, id), &po)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Invoice berhasil dibuat di finance-service, tetapi gagal memperbarui status purchase order lokal")
		return
	}

	h.events.Publish("purchasing.order.invoiced", newAuditEvent("purchasing.order.invoiced", actor, &po.CompanyID, "invoice", "purchase_order", po.ID, po))
	writeJSON(w, http.StatusOK, po)
}
