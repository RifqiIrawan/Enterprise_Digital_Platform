// Package sourcedb menyimpan koneksi read-only ke database Postgres milik
// modul lain (finance_service, sales_service, warehouse_service). dw-service
// TIDAK punya database sendiri -- ini satu-satunya modul yang membaca
// langsung dari database service lain (bukan lewat HTTP API mereka),
// pengecualian yang disengaja untuk bulk analytical extraction, bukan
// pelanggaran diam-diam terhadap prinsip "satu database per service" --
// dw-service tidak pernah menulis ke database ini, hanya SELECT.
package sourcedb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Pools struct {
	Finance   *pgxpool.Pool
	Sales     *pgxpool.Pool
	Warehouse *pgxpool.Pool
}

func Connect(ctx context.Context, financeURL, salesURL, warehouseURL string) (*Pools, error) {
	finance, err := pgxpool.New(ctx, financeURL)
	if err != nil {
		return nil, fmt.Errorf("connect finance source db: %w", err)
	}
	sales, err := pgxpool.New(ctx, salesURL)
	if err != nil {
		return nil, fmt.Errorf("connect sales source db: %w", err)
	}
	warehouse, err := pgxpool.New(ctx, warehouseURL)
	if err != nil {
		return nil, fmt.Errorf("connect warehouse source db: %w", err)
	}
	return &Pools{Finance: finance, Sales: sales, Warehouse: warehouse}, nil
}

func (p *Pools) Close() {
	if p == nil {
		return
	}
	if p.Finance != nil {
		p.Finance.Close()
	}
	if p.Sales != nil {
		p.Sales.Close()
	}
	if p.Warehouse != nil {
		p.Warehouse.Close()
	}
}
