package datalake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

// Gold adalah lapisan ketiga: agregat siap-pakai yang dihitung dari Silver,
// bukan dari Bronze. Membaca Bronze langsung akan menghitung baris yang sama
// berkali-kali; itulah alasan Silver ada.
//
// Aturan bisnisnya SENGAJA identik dengan query ClickHouse yang sudah ada
// (MonthlyFinanceSummary, MonthlySalesSummary): dua tempat yang menjawab
// "berapa penjualan bulan ini" dengan dua angka berbeda lebih buruk daripada
// tidak punya salah satunya. gold_test.go membandingkan keduanya angka demi
// angka.
//
// Hanya tiga dataset -- semuanya punya padanan ClickHouse untuk dibandingkan.
// Dataset Gold lain ditambahkan dengan pola yang sama, bersama padanannya.

const goldPrefix = "gold/"

const (
	GoldFinanceMonthly   = "finance_monthly"
	GoldSalesMonthly     = "sales_monthly"
	GoldInventoryMonthly = "inventory_monthly"
	GoldDowntimeMonthly  = "downtime_monthly"
)

type InventoryMonthlyRow struct {
	CompanyID string          `json:"company_id"`
	Month     string          `json:"month"` // YYYY-MM-01
	StockIn   decimal.Decimal `json:"stock_in"`
	StockOut  decimal.Decimal `json:"stock_out"`
}

type FinanceMonthlyRow struct {
	CompanyID string          `json:"company_id"`
	Month     string          `json:"month"` // YYYY-MM-01
	Revenue   decimal.Decimal `json:"revenue"`
	Expense   decimal.Decimal `json:"expense"`
}

type SalesMonthlyRow struct {
	CompanyID  string          `json:"company_id"`
	Month      string          `json:"month"` // YYYY-MM-01
	SalesValue decimal.Decimal `json:"sales_value"`
}

// DowntimeMonthlyRow: satu baris per (company, bulan, alasan), bukan total
// bulanan, supaya komposisi alasan tidak hilang di Gold.
type DowntimeMonthlyRow struct {
	CompanyID  string `json:"company_id"`
	Month      string `json:"month"` // YYYY-MM-01
	ReasonCode string `json:"reason_code"`
	Minutes    int64  `json:"minutes"`
}

type GoldStats struct {
	Dataset string `json:"dataset"`
	Rows    int    `json:"rows"`
}

func goldKey(dataset string) string { return goldPrefix + dataset + ".jsonl" }

// monthOf mengambil YYYY-MM-01 dari tanggal RFC3339 apa adanya (tanpa
// konversi zona waktu): kolom ClickHouse-nya bertipe Date, jadi tanggal yang
// tertulis di Bronze SUDAH tanggal kalendernya.
func monthOf(date string) (string, bool) {
	if len(date) < 7 || date[4] != '-' {
		return "", false
	}
	return date[:7] + "-01", true
}

// BuildGold menghitung ulang kedua dataset Gold dari Silver yang ada saat
// ini. Silver harus dibangun dulu (BuildAll melakukannya berurutan).
func (c *Client) BuildGold(ctx context.Context) ([]GoldStats, error) {
	fin, err := c.buildFinanceMonthly(ctx)
	if err != nil {
		return nil, err
	}
	sales, err := c.buildSalesMonthly(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := c.buildInventoryMonthly(ctx)
	if err != nil {
		return nil, err
	}
	down, err := c.buildDowntimeMonthly(ctx)
	if err != nil {
		return nil, err
	}
	return []GoldStats{
		{Dataset: GoldFinanceMonthly, Rows: fin},
		{Dataset: GoldSalesMonthly, Rows: sales},
		{Dataset: GoldInventoryMonthly, Rows: inv},
		{Dataset: GoldDowntimeMonthly, Rows: down},
	}, nil
}

type monthKey struct{ company, month string }

func (c *Client) buildFinanceMonthly(ctx context.Context) (int, error) {
	agg := map[monthKey]*FinanceMonthlyRow{}
	// Silver dibaca sebagai aliran: yang ditahan di memori hanya agregat per
	// bulan, bukan seluruh baris.
	err := c.EachSilver(ctx, "finance_journal_lines", func(l []byte) error {
		var r struct {
			CompanyID    string
			EntryDate    string
			EntryStatus  string
			AccountType  string
			DebitAmount  float64
			CreditAmount float64
		}
		if err := json.Unmarshal(l, &r); err != nil {
			return fmt.Errorf("gold finance: baris Silver rusak: %w", err)
		}
		// Hanya jurnal POSTED, revenue = kredit akun REVENUE, expense = debit
		// akun EXPENSE -- sama dengan mv_finance_monthly_line_state.
		if r.EntryStatus != "POSTED" {
			return nil
		}
		month, ok := monthOf(r.EntryDate)
		if !ok {
			return nil
		}
		k := monthKey{r.CompanyID, month}
		row := agg[k]
		if row == nil {
			row = &FinanceMonthlyRow{CompanyID: r.CompanyID, Month: month}
			agg[k] = row
		}
		if r.AccountType == "REVENUE" {
			row.Revenue = row.Revenue.Add(decimal.NewFromFloat(r.CreditAmount))
		}
		if r.AccountType == "EXPENSE" {
			row.Expense = row.Expense.Add(decimal.NewFromFloat(r.DebitAmount))
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	keys := sortedMonthKeys(agg)
	var buf bytes.Buffer
	for _, k := range keys {
		row := agg[k]
		row.Revenue, row.Expense = row.Revenue.Round(2), row.Expense.Round(2)
		b, _ := json.Marshal(row)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return len(keys), c.put(ctx, goldKey(GoldFinanceMonthly), buf.Bytes())
}

func (c *Client) buildSalesMonthly(ctx context.Context) (int, error) {
	agg := map[monthKey]*SalesMonthlyRow{}
	err := c.EachSilver(ctx, "sales_order_lines", func(l []byte) error {
		var r struct {
			CompanyID   string
			OrderDate   string
			OrderStatus string
			Amount      float64
		}
		if err := json.Unmarshal(l, &r); err != nil {
			return fmt.Errorf("gold sales: baris Silver rusak: %w", err)
		}
		// DRAFT belum komitmen, CANCELLED tidak terjadi -- sama dengan
		// MonthlySalesSummary.
		if r.OrderStatus == "DRAFT" || r.OrderStatus == "CANCELLED" {
			return nil
		}
		month, ok := monthOf(r.OrderDate)
		if !ok {
			return nil
		}
		k := monthKey{r.CompanyID, month}
		row := agg[k]
		if row == nil {
			row = &SalesMonthlyRow{CompanyID: r.CompanyID, Month: month}
			agg[k] = row
		}
		row.SalesValue = row.SalesValue.Add(decimal.NewFromFloat(r.Amount))
		return nil
	})
	if err != nil {
		return 0, err
	}

	keys := sortedMonthKeys(agg)
	var buf bytes.Buffer
	for _, k := range keys {
		row := agg[k]
		row.SalesValue = row.SalesValue.Round(2)
		b, _ := json.Marshal(row)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return len(keys), c.put(ctx, goldKey(GoldSalesMonthly), buf.Bytes())
}

func (c *Client) buildInventoryMonthly(ctx context.Context) (int, error) {
	agg := map[monthKey]*InventoryMonthlyRow{}
	err := c.EachSilver(ctx, "inventory_movements", func(l []byte) error {
		var r struct {
			CompanyID    string
			MovementDate string
			MovementType string
			Quantity     float64
		}
		if err := json.Unmarshal(l, &r); err != nil {
			return fmt.Errorf("gold inventory: baris Silver rusak: %w", err)
		}
		// Tanpa kolom status: setiap baris adalah pergerakan yang sudah
		// terjadi. Tipe selain IN/OUT tidak dihitung -- sama dengan
		// MonthlyStockMovementSummary.
		if r.MovementType != "IN" && r.MovementType != "OUT" {
			return nil
		}
		month, ok := monthOf(r.MovementDate)
		if !ok {
			return nil
		}
		k := monthKey{r.CompanyID, month}
		row := agg[k]
		if row == nil {
			row = &InventoryMonthlyRow{CompanyID: r.CompanyID, Month: month}
			agg[k] = row
		}
		q := decimal.NewFromFloat(r.Quantity)
		if r.MovementType == "IN" {
			row.StockIn = row.StockIn.Add(q)
		} else {
			row.StockOut = row.StockOut.Add(q)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	keys := sortedMonthKeys(agg)
	var buf bytes.Buffer
	for _, k := range keys {
		row := agg[k]
		row.StockIn, row.StockOut = row.StockIn.Round(2), row.StockOut.Round(2)
		b, _ := json.Marshal(row)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return len(keys), c.put(ctx, goldKey(GoldInventoryMonthly), buf.Bytes())
}

func sortedMonthKeys[V any](m map[monthKey]V) []monthKey {
	keys := make([]monthKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].company != keys[j].company {
			return keys[i].company < keys[j].company
		}
		return keys[i].month < keys[j].month
	})
	return keys
}

// ReadGold mengembalikan baris-baris satu dataset Gold.
func (c *Client) ReadGold(ctx context.Context, dataset string) ([][]byte, error) {
	return c.readLines(ctx, goldKey(dataset))
}

// BuildAll membangun Silver semua fact lalu Gold. Satu fact yang gagal
// dicatat di hasilnya dan TIDAK menghentikan fact lain -- pola yang sama
// dengan RunSync -- tapi Gold tetap dibangun dari Silver yang ada, jadi
// pemanggil harus membaca errors sebelum mempercayai angka Gold.
type BuildResult struct {
	Silver []SilverStats `json:"silver"`
	Gold   []GoldStats   `json:"gold"`
	Errors []string      `json:"errors,omitempty"`
}

// BuildAll: live berisi LiveKeys per fact (boleh nil, atau tanpa entri untuk
// fact tertentu = fact itu tidak dipangkas).
// full=true memaksa build Silver penuh dari seluruh Bronze (lihat BuildSilver).
func (c *Client) BuildAll(ctx context.Context, live map[string]LiveKeys, full bool) BuildResult {
	var res BuildResult
	facts := make([]string, 0, len(SilverFacts))
	for f := range SilverFacts {
		facts = append(facts, f)
	}
	sort.Strings(facts)
	for _, f := range facts {
		s, err := c.BuildSilver(ctx, f, live[f], full)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("silver %s: %v", f, err))
			continue
		}
		res.Silver = append(res.Silver, s)
	}
	g, err := c.BuildGold(ctx)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("gold: %v", err))
	}
	res.Gold = g
	return res
}

func (c *Client) buildDowntimeMonthly(ctx context.Context) (int, error) {
	type key struct{ company, month, reason string }
	agg := map[key]*DowntimeMonthlyRow{}
	err := c.EachSilver(ctx, "production_downtime", func(l []byte) error {
		var r struct {
			CompanyID  string
			RunDate    string
			ReasonCode string
			Minutes    int64
		}
		if err := json.Unmarshal(l, &r); err != nil {
			return fmt.Errorf("gold downtime: baris Silver rusak: %w", err)
		}
		month, ok := monthOf(r.RunDate)
		if !ok {
			return nil
		}
		k := key{r.CompanyID, month, r.ReasonCode}
		row := agg[k]
		if row == nil {
			row = &DowntimeMonthlyRow{CompanyID: r.CompanyID, Month: month, ReasonCode: r.ReasonCode}
			agg[k] = row
		}
		row.Minutes += r.Minutes
		return nil
	})
	if err != nil {
		return 0, err
	}

	keys := make([]key, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.company != b.company {
			return a.company < b.company
		}
		if a.month != b.month {
			return a.month < b.month
		}
		return a.reason < b.reason
	})
	var buf bytes.Buffer
	for _, k := range keys {
		b, _ := json.Marshal(agg[k])
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return len(keys), c.put(ctx, goldKey(GoldDowntimeMonthly), buf.Bytes())
}
