package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/production-service/internal/model"
)

const bomColumns = `id, company_id, branch_id, bom_code, name, product_id, is_active, created_at, updated_at`

func scanBOM(row pgx.Row, b *model.BillOfMaterial) error {
	return row.Scan(&b.ID, &b.CompanyID, &b.BranchID, &b.BOMCode, &b.Name, &b.ProductID, &b.IsActive, &b.CreatedAt, &b.UpdatedAt)
}

func (h *Handler) listBOMs(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT `+bomColumns+` FROM bill_of_materials WHERE company_id = $1 ORDER BY bom_code ASC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data BOM")
		return
	}
	defer rows.Close()

	boms := []model.BillOfMaterial{}
	for rows.Next() {
		var b model.BillOfMaterial
		if err := scanBOM(rows, &b); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data BOM")
			return
		}
		boms = append(boms, b)
	}
	writeJSON(w, http.StatusOK, boms)
}

type bomLineInput struct {
	ComponentProductID string  `json:"component_product_id"`
	QuantityPerUnit    float64 `json:"quantity_per_unit"`
}

type createBOMRequest struct {
	CompanyID string         `json:"company_id"`
	BranchID  *string        `json:"branch_id"`
	BOMCode   string         `json:"bom_code"`
	Name      string         `json:"name"`
	ProductID string         `json:"product_id"`
	Lines     []bomLineInput `json:"lines"`
}

type bomWithLines struct {
	model.BillOfMaterial
	Lines []model.BOMLine `json:"lines"`
}

func (h *Handler) createBOM(w http.ResponseWriter, r *http.Request) {
	var req createBOMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.BOMCode = strings.TrimSpace(req.BOMCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.BOMCode == "" || req.Name == "" || req.ProductID == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, bom_code, name, product_id, dan minimal 1 komponen wajib diisi")
		return
	}
	for _, l := range req.Lines {
		if l.ComponentProductID == "" || l.QuantityPerUnit <= 0 {
			writeError(w, http.StatusBadRequest, "Setiap komponen wajib punya component_product_id dan quantity_per_unit > 0")
			return
		}
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var b model.BillOfMaterial
	err = scanBOM(tx.QueryRow(ctx, `
		INSERT INTO bill_of_materials (company_id, branch_id, bom_code, name, product_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+bomColumns,
		req.CompanyID, req.BranchID, req.BOMCode, req.Name, req.ProductID,
	), &b)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode BOM sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat BOM")
		return
	}

	lines := make([]model.BOMLine, 0, len(req.Lines))
	for i, l := range req.Lines {
		var line model.BOMLine
		err := tx.QueryRow(ctx, `
			INSERT INTO bom_lines (bom_id, line_number, component_product_id, quantity_per_unit)
			VALUES ($1, $2, $3, $4)
			RETURNING id, bom_id, line_number, component_product_id, quantity_per_unit`,
			b.ID, i+1, l.ComponentProductID, l.QuantityPerUnit,
		).Scan(&line.ID, &line.BOMID, &line.LineNumber, &line.ComponentProductID, &line.QuantityPerUnit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris komponen BOM")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan BOM")
		return
	}

	h.events.Publish("production.bom.created", newAuditEvent("production.bom.created", actorFromHeader(r), &b.CompanyID, "create", "bom", b.ID, b))
	writeJSON(w, http.StatusCreated, bomWithLines{BillOfMaterial: b, Lines: lines})
}

func (h *Handler) fetchBOMLines(ctx context.Context, bomID string) ([]model.BOMLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, bom_id, line_number, component_product_id, quantity_per_unit
		FROM bom_lines WHERE bom_id = $1 ORDER BY line_number ASC`, bomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.BOMLine{}
	for rows.Next() {
		var l model.BOMLine
		if err := rows.Scan(&l.ID, &l.BOMID, &l.LineNumber, &l.ComponentProductID, &l.QuantityPerUnit); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (h *Handler) getBOM(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var b model.BillOfMaterial
	err := scanBOM(h.pool.QueryRow(ctx, `SELECT `+bomColumns+` FROM bill_of_materials WHERE id = $1`, id), &b)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "BOM tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat BOM")
		return
	}

	lines, err := h.fetchBOMLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris komponen BOM")
		return
	}
	writeJSON(w, http.StatusOK, bomWithLines{BillOfMaterial: b, Lines: lines})
}

type updateBOMRequest struct {
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

func (h *Handler) updateBOM(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateBOMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}

	var b model.BillOfMaterial
	err := scanBOM(h.pool.QueryRow(r.Context(), `
		UPDATE bill_of_materials SET name = $1, is_active = $2, updated_at = now()
		WHERE id = $3
		RETURNING `+bomColumns,
		req.Name, req.IsActive, id,
	), &b)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "BOM tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui BOM")
		return
	}

	h.events.Publish("production.bom.updated", newAuditEvent("production.bom.updated", actorFromHeader(r), &b.CompanyID, "update", "bom", b.ID, b))
	writeJSON(w, http.StatusOK, b)
}
