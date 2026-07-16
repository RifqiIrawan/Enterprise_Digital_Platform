package etl

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

const financeSourceTable = "finance_journal_lines"

const financeExtractSQL = `
	SELECT jl.id, jl.journal_id, je.company_id, je.branch_id, je.entry_number, je.entry_date,
	       je.period, je.reference_type, je.status, jl.account_id, a.account_code, a.account_name,
	       a.account_type, jl.debit_amount, jl.credit_amount, je.posted_at,
	       COALESCE(je.posted_at, je.created_at) AS watermark
	FROM journal_lines jl
	JOIN journal_entries je ON je.id = jl.journal_id
	JOIN accounts a ON a.id = jl.account_id
	WHERE COALESCE(je.posted_at, je.created_at) >= $1
	ORDER BY COALESCE(je.posted_at, je.created_at)`

// SyncFinance mengekstrak journal_lines (di-join ke journal_entries dan
// accounts) dari finance_service, lalu load ke fact_finance_journal_lines di
// ClickHouse. Watermark dari journal_lines/journal_entries sengaja BUKAN
// updated_at (tabel itu tidak punya kolom itu -- journal_lines immutable
// setelah dibuat, journal_entries cuma berubah lewat status DRAFT->POSTED
// yang tercermin di posted_at) -- lihat komentar di migrations/001_init.sql
// finance-service, dikonfirmasi langsung sebelum menulis SQL ini.
func SyncFinance(ctx context.Context, source *pgxpool.Pool, dest *ch.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, financeSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get finance watermark: %w", err)
	}

	rows, err := source.Query(ctx, financeExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract finance rows: %w", err)
	}
	defer rows.Close()

	var out []ch.FinanceJournalLineRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.FinanceJournalLineRow
		var wm time.Time
		if err := rows.Scan(
			&r.LineID, &r.JournalID, &r.CompanyID, &r.BranchID, &r.EntryNumber, &r.EntryDate,
			&r.Period, &r.ReferenceType, &r.EntryStatus, &r.AccountID, &r.AccountCode, &r.AccountName,
			&r.AccountType, &r.DebitAmount, &r.CreditAmount, &r.PostedAt, &wm,
		); err != nil {
			return 0, fmt.Errorf("scan finance row: %w", err)
		}
		out = append(out, r)
		if wm.After(maxWatermark) {
			maxWatermark = wm
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate finance rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertFinanceJournalLines(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load finance rows: %w", err)
	}
	if err := dest.SetWatermark(ctx, financeSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance finance watermark: %w", err)
	}
	return len(out), nil
}
