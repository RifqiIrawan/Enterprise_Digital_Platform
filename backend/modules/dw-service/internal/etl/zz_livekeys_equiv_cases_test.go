package etl

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

// liveKeysStreamed adalah cara LAMA menghitung kunci hidup: alirkan seluruh baris
// ekstrak sync dan hitung kunci tiap baris dengan datalake.RowKey. Sekarang
// hanya dipakai sebagai acuan: TestKeysOnly_MatchesStreamedExtract menuntut
// query kunci-saja menghasilkan himpunan yang sama persis.
func liveKeysStreamed[T any](fact string, each func(func(T) error) error) (*datalake.KeySet, error) {
	keys := datalake.NewKeySet(0)
	err := each(func(r T) error {
		k, ok := datalake.RowKey(fact, r)
		if !ok {
			return fmt.Errorf("live keys %s: baris sumber tanpa kunci yang valid", fact)
		}
		keys.Add(k)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

type liveKeysCase struct {
	name     string
	keysOnly func(context.Context, *pgxpool.Pool) (*datalake.KeySet, error)
	streamed func(context.Context, *pgxpool.Pool) (*datalake.KeySet, error)
}

var liveKeysCases = []liveKeysCase{
	{
		name:     assetSourceTable,
		keysOnly: LiveKeysAsset,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(assetSourceTable, func(fn func(ch.AssetMaintenanceRow) error) error {
				_, err := eachAsset(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     crmSourceTable,
		keysOnly: LiveKeysCRM,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(crmSourceTable, func(fn func(ch.CRMOpportunityRow) error) error {
				_, err := eachCRM(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     ecommerceSourceTable,
		keysOnly: LiveKeysEcommerce,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(ecommerceSourceTable, func(fn func(ch.EcommerceOrderLineRow) error) error {
				_, err := eachEcommerce(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     financeSourceTable,
		keysOnly: LiveKeysFinance,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(financeSourceTable, func(fn func(ch.FinanceJournalLineRow) error) error {
				_, err := eachFinance(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     fleetSourceTable,
		keysOnly: LiveKeysFleet,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(fleetSourceTable, func(fn func(ch.FleetDeliveryOrderRow) error) error {
				_, err := eachFleet(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     hrSourceTable,
		keysOnly: LiveKeysHR,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(hrSourceTable, func(fn func(ch.HRPayrollDetailRow) error) error {
				_, err := eachHR(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     hrLeaveSourceTable,
		keysOnly: LiveKeysHRLeave,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(hrLeaveSourceTable, func(fn func(ch.HRLeaveRow) error) error {
				_, err := eachHRLeave(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     hrKPISourceTable,
		keysOnly: LiveKeysHRKPI,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(hrKPISourceTable, func(fn func(ch.HRKPIReviewRow) error) error {
				_, err := eachHRKPI(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     inventorySourceTable,
		keysOnly: LiveKeysInventory,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(inventorySourceTable, func(fn func(ch.InventoryMovementRow) error) error {
				_, err := eachInventory(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     iotSourceTable,
		keysOnly: LiveKeysIoT,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(iotSourceTable, func(fn func(ch.IoTReadingRow) error) error {
				_, err := eachIoT(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     productionSourceTable,
		keysOnly: LiveKeysProduction,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(productionSourceTable, func(fn func(ch.ProductionWorkOrderRow) error) error {
				_, err := eachProduction(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     productionDowntimeSourceTable,
		keysOnly: LiveKeysProductionDowntime,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(productionDowntimeSourceTable, func(fn func(ch.ProductionDowntimeRow) error) error {
				_, err := eachProductionDowntime(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     productionOEESourceTable,
		keysOnly: LiveKeysProductionOEE,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(productionOEESourceTable, func(fn func(ch.ProductionOEERow) error) error {
				_, err := eachProductionOEE(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     projectSourceTable,
		keysOnly: LiveKeysProject,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(projectSourceTable, func(fn func(ch.ProjectTimesheetRow) error) error {
				_, err := eachProject(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     purchasingSourceTable,
		keysOnly: LiveKeysPurchasing,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(purchasingSourceTable, func(fn func(ch.PurchasingOrderLineRow) error) error {
				_, err := eachPurchasing(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     qcSourceTable,
		keysOnly: LiveKeysQC,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(qcSourceTable, func(fn func(ch.QCInspectionRow) error) error {
				_, err := eachQC(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     salesSourceTable,
		keysOnly: LiveKeysSales,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(salesSourceTable, func(fn func(ch.SalesOrderLineRow) error) error {
				_, err := eachSales(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
	{
		name:     ticketingSourceTable,
		keysOnly: LiveKeysTicketing,
		streamed: func(ctx context.Context, p *pgxpool.Pool) (*datalake.KeySet, error) {
			return liveKeysStreamed(ticketingSourceTable, func(fn func(ch.TicketingTicketRow) error) error {
				_, err := eachTicketing(ctx, p, time.Time{}, fn)
				return err
			})
		},
	},
}
