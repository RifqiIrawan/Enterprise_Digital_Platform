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

const ticketingSourceTable = "tickets"

// ticketingExtractSQL watermark pakai tickets.updated_at -- grain di sini
// SATU baris per tiket (bukan line-level dari tabel anak), jadi tidak perlu
// join balik ke parent seperti fact sales/purchasing/asset. resolved_at/
// closed_at disimpan RAW (bukan dihitung jadi resolution_minutes) supaya
// perhitungan durasi bisa dilakukan di query analitik masa depan kalau
// terbukti perlu, bukan dibekukan di sini.
const ticketingExtractSQL = `
	SELECT t.id, t.company_id, t.branch_id, t.ticket_number, t.category_id, c.name AS category_name,
	       t.subject, t.priority, t.status, t.requester_name, t.requester_email, t.assigned_to,
	       t.created_at, t.resolved_at, t.closed_at, t.updated_at
	FROM tickets t
	JOIN ticket_categories c ON c.id = t.category_id
	WHERE t.updated_at >= $1
	ORDER BY t.updated_at`

// SyncTicketing mengekstrak tickets (di-join ke ticket_categories) dari
// ticketing-service, lalu load ke fact_ticketing_tickets di ClickHouse.
// ticket_comments SENGAJA tidak dimodelkan sebagai fact -- volume aktivitas
// murni tanpa measure numerik.
func SyncTicketing(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, ticketingSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get ticketing watermark: %w", err)
	}

	rows, err := source.Query(ctx, ticketingExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract ticketing rows: %w", err)
	}
	defer rows.Close()

	var out []ch.TicketingTicketRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.TicketingTicketRow
		if err := rows.Scan(
			&r.TicketID, &r.CompanyID, &r.BranchID, &r.TicketNumber, &r.CategoryID, &r.CategoryName,
			&r.Subject, &r.Priority, &r.Status, &r.RequesterName, &r.RequesterEmail, &r.AssignedTo,
			&r.CreatedAt, &r.ResolvedAt, &r.ClosedAt, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan ticketing row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate ticketing rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertTicketingTickets(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load ticketing rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, ticketingSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", ticketingSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, ticketingSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance ticketing watermark: %w", err)
	}
	return len(out), nil
}
