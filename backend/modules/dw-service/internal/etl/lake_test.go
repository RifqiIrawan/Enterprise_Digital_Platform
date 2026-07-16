package etl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

// TestSyncFinance_WritesToDataLake memverifikasi dual-write dw-service ke
// MinIO (bronze layer) SUNGGUHAN lewat SyncFinance asli -- bukan cuma
// datalake package terisolasi (lihat internal/datalake/datalake_test.go
// untuk itu). Guard MinIO-nya SENGAJA di dalam test ini (t.Skip), BUKAN di
// TestMain package ini -- MinIO adalah side-channel opsional (lake boleh
// nil, lihat komentar SyncFinance's lake param), jadi 10 test lain di
// package ini TIDAK boleh ikut ter-skip kalau cuma MinIO yang tidak
// tersedia (mereka sudah pass dengan lake=nil).
func TestSyncFinance_WritesToDataLake(t *testing.T) {
	ctx := context.Background()
	lake, err := datalake.Connect(ctx, getEnv("DW_TEST_MINIO_ENDPOINT", "localhost:9004"),
		getEnv("DW_TEST_MINIO_ACCESS_KEY", "minioadmin"),
		getEnv("DW_TEST_MINIO_SECRET_KEY", "minioadmin"),
		getEnv("DW_TEST_MINIO_BUCKET", "dw-lake-test"), false)
	if err != nil {
		t.Skipf("SKIP: datalake tests need a local MinIO: %v", err)
	}

	companyID := uuid.New()
	lineID, accountCode := mustSeedJournalEntryWithLine(t, companyID, "POSTED")

	if _, err := SyncFinance(ctx, sourcePool, chClient, lake); err != nil {
		t.Fatalf("SyncFinance: %v", err)
	}

	keys, err := lake.ListKeys(ctx, financeSourceTable+"/")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) == 0 {
		t.Fatalf("expected at least 1 object under %s/, got none", financeSourceTable)
	}

	// baris yang baru di-seed pasti ada di object PALING BARU (key
	// mengandung syncedAt.UnixNano(), lexicographically terurut sama
	// dengan urutan waktu untuk timestamp di rentang tahun yang sama).
	latestKey := keys[len(keys)-1]
	data, err := lake.Get(ctx, latestKey)
	if err != nil {
		t.Fatalf("Get %s: %v", latestKey, err)
	}

	found := false
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		var row struct {
			LineID      uuid.UUID
			AccountCode string
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("unmarshal lake line %q: %v", line, err)
		}
		if row.LineID == lineID {
			found = true
			if row.AccountCode != accountCode {
				t.Errorf("lake row account_code = %q, want %q", row.AccountCode, accountCode)
			}
		}
	}
	if !found {
		t.Errorf("line_id %s not found in latest lake object %s", lineID, latestKey)
	}
}
