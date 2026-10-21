package etl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

// keysOnlySQL membuat query kunci-saja dari SQL ekstrak sebuah fact: daftar
// SELECT diganti dua kolom (id baris dan company_id), sedangkan FROM, JOIN, dan
// WHERE dipakai APA ADANYA. Itu intinya: "baris yang masih ada" harus berarti
// "baris yang masih akan disalin sync", jadi filter tidak boleh ditulis dua
// kali. Query kunci yang filternya menyimpang memangkas baris yang masih ada
// (terlalu ketat) atau membiarkan baris hantu (terlalu longgar), dan tidak ada
// galat yang menandainya -- itu sebabnya TestKeysOnly_MatchesStreamedExtract
// membandingkan hasilnya dengan ekstrak penuh untuk setiap fact.
//
// Kolom pertama SELALU id baris; companyCol adalah indeks company_id dalam
// daftar SELECT (0-based). ORDER BY dibuang: urutan tidak mengubah himpunan
// kunci, dan mengurutkan jutaan baris hanya untuk dibuang itu pemborosan.
func keysOnlySQL(extractSQL string, companyCol int) (string, error) {
	upper := strings.ToUpper(extractSQL)
	selectAt := strings.Index(upper, "SELECT")
	if selectAt < 0 {
		return "", fmt.Errorf("keysOnlySQL: tidak ada SELECT")
	}
	listStart := selectAt + len("SELECT")

	// Cari FROM pertama di kedalaman kurung nol: subquery/COALESCE di dalam
	// daftar SELECT boleh punya FROM sendiri.
	depth, fromAt := 0, -1
	for i := listStart; i < len(extractSQL) && fromAt < 0; i++ {
		switch extractSQL[i] {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 && isKeywordAt(upper, i, "FROM") {
				fromAt = i
			}
		}
	}
	if fromAt < 0 {
		return "", fmt.Errorf("keysOnlySQL: tidak ada FROM")
	}

	columns := splitTopLevel(extractSQL[listStart:fromAt])
	if companyCol <= 0 || companyCol >= len(columns) {
		return "", fmt.Errorf("keysOnlySQL: companyCol %d di luar %d kolom", companyCol, len(columns))
	}
	rest := extractSQL[fromAt:]
	if at := strings.LastIndex(strings.ToUpper(rest), "ORDER BY"); at >= 0 {
		rest = rest[:at]
	}
	return "SELECT " + stripAlias(columns[0]) + ", " + stripAlias(columns[companyCol]) + "\n\t" + strings.TrimRight(rest, " \t\n"), nil
}

func isKeywordAt(upper string, i int, word string) bool {
	if !strings.HasPrefix(upper[i:], word) {
		return false
	}
	if i > 0 {
		if c := upper[i-1]; c != ' ' && c != '\n' && c != '\t' && c != ')' {
			return false
		}
	}
	end := i + len(word)
	return end >= len(upper) || upper[end] == ' ' || upper[end] == '\n' || upper[end] == '\t' || upper[end] == '('
}

// splitTopLevel memecah daftar kolom pada koma di kedalaman kurung nol.
func splitTopLevel(list string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(list); i++ {
		switch list[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(list[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(list[start:]))
}

// stripAlias membuang " AS nama" di ujung ekspresi kolom.
func stripAlias(col string) string {
	if at := strings.LastIndex(strings.ToUpper(col), " AS "); at >= 0 {
		return strings.TrimSpace(col[:at])
	}
	return col
}

// liveKeysOnly mengambil kunci Silver ("<company_id>/<id>") semua baris yang
// masih akan disalin sync, hanya dengan dua kolom dari Postgres. Aturan kunci
// sama dengan datalake.RowKey: id atau company_id kosong/nol adalah galat
// (daftar yang bolong akan memangkas baris yang masih ada).
func liveKeysOnly(ctx context.Context, source *pgxpool.Pool, fact, extractSQL string, companyCol int) (*datalake.KeySet, error) {
	query, err := keysOnlySQL(extractSQL, companyCol)
	if err != nil {
		return nil, fmt.Errorf("live keys %s: %w", fact, err)
	}
	rows, err := source.Query(ctx, query, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("live keys %s: %w", fact, err)
	}
	defer rows.Close()

	keys := datalake.NewKeySet(0)
	for rows.Next() {
		var id, company uuid.UUID
		if err := rows.Scan(&id, &company); err != nil {
			return nil, fmt.Errorf("live keys %s: %w", fact, err)
		}
		if id == uuid.Nil || company == uuid.Nil {
			return nil, fmt.Errorf("live keys %s: baris sumber tanpa kunci yang valid", fact)
		}
		keys.Add(company.String() + "/" + id.String())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("live keys %s: %w", fact, err)
	}
	return keys, nil
}
