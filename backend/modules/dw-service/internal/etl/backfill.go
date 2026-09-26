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
