package etl

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

const crmSourceTable = "opportunities"

// crmExtractSQL watermark pakai opportunities.updated_at, mengikuti pola
// SyncSales (fact_sales_order_lines) -- opportunity yang berubah stage
// (mis. WON/LOST) ikut ter-extract ulang lewat updated_at-nya sendiri,
// bukan lewat tabel parent seperti sales/purchasing/asset karena grain di
// sini SATU baris per opportunity (bukan line-level dari tabel anak).
const crmExtractSQL = `
	SELECT o.id, o.company_id, o.branch_id, o.opportunity_number, o.account_id, a.name AS account_name,
	       o.contact_id, o.name AS opportunity_name, o.stage, o.amount, o.probability,
	       o.expected_close_date, o.owner_user_id, o.created_at, o.updated_at
	FROM opportunities o
	JOIN accounts a ON a.id = o.account_id
	WHERE o.updated_at >= $1
	ORDER BY o.updated_at`

// SyncCRM mengekstrak opportunities (di-join ke accounts) dari crm-service,
// lalu load ke fact_crm_opportunities di ClickHouse. Leads/Contacts/
// Activities SENGAJA tidak dimodelkan sebagai fact -- tidak ada measure
// numerik yang natural di sana (lihat catatan desain di NEXT_SESSION.md).
func SyncCRM(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, crmSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get crm watermark: %w", err)
	}

	rows, err := source.Query(ctx, crmExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract crm rows: %w", err)
	}
	defer rows.Close()

	var out []ch.CRMOpportunityRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.CRMOpportunityRow
		if err := rows.Scan(
			&r.OpportunityID, &r.CompanyID, &r.BranchID, &r.OpportunityNumber, &r.AccountID, &r.AccountName,
			&r.ContactID, &r.OpportunityName, &r.Stage, &r.Amount, &r.Probability,
			&r.ExpectedCloseDate, &r.OwnerUserID, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan crm row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate crm rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertCRMOpportunities(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load crm rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, crmSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", crmSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, crmSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance crm watermark: %w", err)
	}
	return len(out), nil
}
