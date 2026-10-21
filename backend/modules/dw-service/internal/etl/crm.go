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

	out, maxWatermark, err := extractCRM(ctx, source, watermark)
	if err != nil {
		return 0, err
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

// extractCRM adalah bagian "tarik dari Postgres" milik SyncCRM, dipisah supaya
// Backfill bisa memakai SQL dan pemetaan kolom yang PERSIS sama (watermark nol
// = seluruh baris) tanpa menyentuh ClickHouse.
// eachCRM menyalurkan baris hasil ekstrak satu per satu ke fn tanpa menampungnya,
// dan mengembalikan watermark terbesar. extractCRM dan daftar kunci hidup sama-sama
// dibangun di atasnya, jadi keduanya selalu memakai query dan baris yang sama.
func eachCRM(ctx context.Context, source *pgxpool.Pool, watermark time.Time, fn func(ch.CRMOpportunityRow) error) (time.Time, error) {
	rows, err := source.Query(ctx, crmExtractSQL, watermark)
	if err != nil {
		return time.Time{}, fmt.Errorf("extract crm rows: %w", err)
	}
	defer rows.Close()

	maxWatermark := watermark
	for rows.Next() {
		var r ch.CRMOpportunityRow
		if err := rows.Scan(
			&r.OpportunityID, &r.CompanyID, &r.BranchID, &r.OpportunityNumber, &r.AccountID, &r.AccountName,
			&r.ContactID, &r.OpportunityName, &r.Stage, &r.Amount, &r.Probability,
			&r.ExpectedCloseDate, &r.OwnerUserID, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return time.Time{}, fmt.Errorf("scan crm row: %w", err)
		}
		if err := fn(r); err != nil {
			return time.Time{}, err
		}
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return time.Time{}, fmt.Errorf("iterate crm rows: %w", err)
	}

	return maxWatermark, nil
}

func extractCRM(ctx context.Context, source *pgxpool.Pool, watermark time.Time) ([]ch.CRMOpportunityRow, time.Time, error) {
	var out []ch.CRMOpportunityRow
	maxWatermark, err := eachCRM(ctx, source, watermark, func(r ch.CRMOpportunityRow) error {
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	return out, maxWatermark, nil
}
