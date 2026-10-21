package etl

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

// BackfillX untuk tiap fact: ekstrak SELURUH baris sumber (watermark nol)
// dengan SQL yang sama persis dengan sync biasa, lalu tulis ke Bronze saja.
// Cara kerja dan alasannya ada di komentar backfill() di facts.go.

func BackfillAsset(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, assetSourceTable, func() ([]ch.AssetMaintenanceRow, error) {
		rows, _, err := extractAsset(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillCRM(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, crmSourceTable, func() ([]ch.CRMOpportunityRow, error) {
		rows, _, err := extractCRM(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillEcommerce(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, ecommerceSourceTable, func() ([]ch.EcommerceOrderLineRow, error) {
		rows, _, err := extractEcommerce(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillFinance(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, financeSourceTable, func() ([]ch.FinanceJournalLineRow, error) {
		rows, _, err := extractFinance(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillFleet(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, fleetSourceTable, func() ([]ch.FleetDeliveryOrderRow, error) {
		rows, _, err := extractFleet(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillHR(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, hrSourceTable, func() ([]ch.HRPayrollDetailRow, error) {
		rows, _, err := extractHR(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillHRLeave(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, hrLeaveSourceTable, func() ([]ch.HRLeaveRow, error) {
		rows, _, err := extractHRLeave(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillHRKPI(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, hrKPISourceTable, func() ([]ch.HRKPIReviewRow, error) {
		rows, _, err := extractHRKPI(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillInventory(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, inventorySourceTable, func() ([]ch.InventoryMovementRow, error) {
		rows, _, err := extractInventory(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillIoT(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, iotSourceTable, func() ([]ch.IoTReadingRow, error) {
		rows, _, err := extractIoT(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillProduction(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, productionSourceTable, func() ([]ch.ProductionWorkOrderRow, error) {
		rows, _, err := extractProduction(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillProductionOEE(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, productionOEESourceTable, func() ([]ch.ProductionOEERow, error) {
		rows, _, err := extractProductionOEE(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillProductionDowntime(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, productionDowntimeSourceTable, func() ([]ch.ProductionDowntimeRow, error) {
		rows, _, err := extractProductionDowntime(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillProject(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, projectSourceTable, func() ([]ch.ProjectTimesheetRow, error) {
		rows, _, err := extractProject(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillPurchasing(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, purchasingSourceTable, func() ([]ch.PurchasingOrderLineRow, error) {
		rows, _, err := extractPurchasing(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillQC(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, qcSourceTable, func() ([]ch.QCInspectionRow, error) {
		rows, _, err := extractQC(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillSales(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, salesSourceTable, func() ([]ch.SalesOrderLineRow, error) {
		rows, _, err := extractSales(ctx, source, time.Time{})
		return rows, err
	})
}

func BackfillTicketing(ctx context.Context, source *pgxpool.Pool, lake *datalake.Client) (int, error) {
	return backfill(ctx, lake, ticketingSourceTable, func() ([]ch.TicketingTicketRow, error) {
		rows, _, err := extractTicketing(ctx, source, time.Time{})
		return rows, err
	})
}

func LiveKeysAsset(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(assetSourceTable, func(fn func(ch.AssetMaintenanceRow) error) error {
		_, err := eachAsset(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysCRM(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(crmSourceTable, func(fn func(ch.CRMOpportunityRow) error) error {
		_, err := eachCRM(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysEcommerce(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(ecommerceSourceTable, func(fn func(ch.EcommerceOrderLineRow) error) error {
		_, err := eachEcommerce(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysFinance(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(financeSourceTable, func(fn func(ch.FinanceJournalLineRow) error) error {
		_, err := eachFinance(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysFleet(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(fleetSourceTable, func(fn func(ch.FleetDeliveryOrderRow) error) error {
		_, err := eachFleet(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysHR(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(hrSourceTable, func(fn func(ch.HRPayrollDetailRow) error) error {
		_, err := eachHR(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysHRLeave(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(hrLeaveSourceTable, func(fn func(ch.HRLeaveRow) error) error {
		_, err := eachHRLeave(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysHRKPI(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(hrKPISourceTable, func(fn func(ch.HRKPIReviewRow) error) error {
		_, err := eachHRKPI(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysInventory(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(inventorySourceTable, func(fn func(ch.InventoryMovementRow) error) error {
		_, err := eachInventory(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysIoT(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(iotSourceTable, func(fn func(ch.IoTReadingRow) error) error {
		_, err := eachIoT(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysProduction(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(productionSourceTable, func(fn func(ch.ProductionWorkOrderRow) error) error {
		_, err := eachProduction(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysProductionDowntime(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(productionDowntimeSourceTable, func(fn func(ch.ProductionDowntimeRow) error) error {
		_, err := eachProductionDowntime(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysProductionOEE(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(productionOEESourceTable, func(fn func(ch.ProductionOEERow) error) error {
		_, err := eachProductionOEE(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysProject(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(projectSourceTable, func(fn func(ch.ProjectTimesheetRow) error) error {
		_, err := eachProject(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysPurchasing(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(purchasingSourceTable, func(fn func(ch.PurchasingOrderLineRow) error) error {
		_, err := eachPurchasing(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysQC(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(qcSourceTable, func(fn func(ch.QCInspectionRow) error) error {
		_, err := eachQC(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysSales(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(salesSourceTable, func(fn func(ch.SalesOrderLineRow) error) error {
		_, err := eachSales(ctx, source, time.Time{}, fn)
		return err
	})
}

func LiveKeysTicketing(ctx context.Context, source *pgxpool.Pool) (*datalake.KeySet, error) {
	return liveKeys(ticketingSourceTable, func(fn func(ch.TicketingTicketRow) error) error {
		_, err := eachTicketing(ctx, source, time.Time{}, fn)
		return err
	})
}
