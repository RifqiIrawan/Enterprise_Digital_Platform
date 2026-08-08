package httpapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// nextSequence generates a period-scoped sequential document number
// (e.g. "LEAD-202607-0001"), same idiom as sales-service's nextSequence
// (backend/modules/sales-service/internal/httpapi/quotations.go) -- copied
// per-module rather than imported, matching this codebase's convention of no
// cross-module Go imports (each service is a fully independent module).
func nextSequence(ctx context.Context, tx pgx.Tx, companyID, table, column, prefix, period string) (string, error) {
	var count int
	likePattern := prefix + "-" + strings.ReplaceAll(period, "-", "") + "-%"
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE company_id = $1 AND %s LIKE $2`, table, column)
	if err := tx.QueryRow(ctx, query, companyID, likePattern).Scan(&count); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%04d", prefix, strings.ReplaceAll(period, "-", ""), count+1), nil
}
