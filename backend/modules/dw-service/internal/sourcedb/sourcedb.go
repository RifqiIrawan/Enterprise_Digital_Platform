// Package sourcedb menyimpan koneksi read-only ke database Postgres milik
// modul lain (finance_service, sales_service, warehouse_service, dst).
// dw-service TIDAK punya database sendiri -- ini satu-satunya modul yang
// membaca langsung dari database service lain (bukan lewat HTTP API
// mereka), pengecualian yang disengaja untuk bulk analytical extraction,
// bukan pelanggaran diam-diam terhadap prinsip "satu database per service"
// -- dw-service tidak pernah menulis ke database ini, hanya SELECT.
package sourcedb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Pools struct {
	Finance    *pgxpool.Pool
	Sales      *pgxpool.Pool
	Warehouse  *pgxpool.Pool
	HR         *pgxpool.Pool
	Purchasing *pgxpool.Pool
	Production *pgxpool.Pool
	QC         *pgxpool.Pool
	Asset      *pgxpool.Pool
	IoT        *pgxpool.Pool
}

// URLs mengumpulkan connection string untuk kesembilan source database --
// dikelompokkan jadi satu struct supaya Connect tidak punya 9 parameter
// posisional yang gampang tertukar urutannya.
type URLs struct {
	Finance    string
	Sales      string
	Warehouse  string
	HR         string
	Purchasing string
	Production string
	QC         string
	Asset      string
	IoT        string
}

func Connect(ctx context.Context, urls URLs) (*Pools, error) {
	finance, err := pgxpool.New(ctx, urls.Finance)
	if err != nil {
		return nil, fmt.Errorf("connect finance source db: %w", err)
	}
	sales, err := pgxpool.New(ctx, urls.Sales)
	if err != nil {
		return nil, fmt.Errorf("connect sales source db: %w", err)
	}
	warehouse, err := pgxpool.New(ctx, urls.Warehouse)
	if err != nil {
		return nil, fmt.Errorf("connect warehouse source db: %w", err)
	}
	hr, err := pgxpool.New(ctx, urls.HR)
	if err != nil {
		return nil, fmt.Errorf("connect hr source db: %w", err)
	}
	purchasing, err := pgxpool.New(ctx, urls.Purchasing)
	if err != nil {
		return nil, fmt.Errorf("connect purchasing source db: %w", err)
	}
	production, err := pgxpool.New(ctx, urls.Production)
	if err != nil {
		return nil, fmt.Errorf("connect production source db: %w", err)
	}
	qc, err := pgxpool.New(ctx, urls.QC)
	if err != nil {
		return nil, fmt.Errorf("connect qc source db: %w", err)
	}
	asset, err := pgxpool.New(ctx, urls.Asset)
	if err != nil {
		return nil, fmt.Errorf("connect asset source db: %w", err)
	}
	iot, err := pgxpool.New(ctx, urls.IoT)
	if err != nil {
		return nil, fmt.Errorf("connect iot source db: %w", err)
	}
	return &Pools{
		Finance: finance, Sales: sales, Warehouse: warehouse,
		HR: hr, Purchasing: purchasing, Production: production,
		QC: qc, Asset: asset, IoT: iot,
	}, nil
}

func (p *Pools) Close() {
	if p == nil {
		return
	}
	for _, pool := range []*pgxpool.Pool{p.Finance, p.Sales, p.Warehouse, p.HR, p.Purchasing, p.Production, p.QC, p.Asset, p.IoT} {
		if pool != nil {
			pool.Close()
		}
	}
}
