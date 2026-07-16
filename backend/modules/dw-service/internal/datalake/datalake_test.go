package datalake

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

var testClient *Client

func TestMain(m *testing.M) {
	ctx := context.Background()

	endpoint := getEnv("DW_TEST_MINIO_ENDPOINT", "localhost:9004")
	accessKey := getEnv("DW_TEST_MINIO_ACCESS_KEY", "minioadmin")
	secretKey := getEnv("DW_TEST_MINIO_SECRET_KEY", "minioadmin")
	bucket := getEnv("DW_TEST_MINIO_BUCKET", "dw-lake-test")

	c, err := Connect(ctx, endpoint, accessKey, secretKey, bucket, false)
	if err != nil {
		fmt.Printf("SKIP: datalake tests need a local MinIO (tried %s): %v\n", endpoint, err)
		os.Exit(0)
	}
	testClient = c

	os.Exit(m.Run())
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type sampleRow struct {
	ID    string
	Value float64
}

func TestWriteJSONLines_RoundTrips(t *testing.T) {
	fact := "test_fact_" + time.Now().Format("150405.000000000")
	rows := []sampleRow{
		{ID: "a", Value: 1.5},
		{ID: "b", Value: 2.5},
	}
	syncedAt := time.Now()

	if err := testClient.WriteJSONLines(context.Background(), fact, rows, syncedAt); err != nil {
		t.Fatalf("WriteJSONLines: %v", err)
	}

	keys, err := testClient.ListKeys(context.Background(), fact+"/")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected exactly 1 object under %s/, got %d: %v", fact, len(keys), keys)
	}

	wantKey := fmt.Sprintf("%s/%04d/%02d/%02d/%d.jsonl", fact, syncedAt.Year(), syncedAt.Month(), syncedAt.Day(), syncedAt.UnixNano())
	if keys[0] != wantKey {
		t.Errorf("key = %q, want %q", keys[0], wantKey)
	}

	data, err := testClient.Get(context.Background(), keys[0])
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSON lines, got %d: %q", len(lines), string(data))
	}
	var got []sampleRow
	for _, line := range lines {
		var r sampleRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("unmarshal line %q: %v", line, err)
		}
		got = append(got, r)
	}
	if got[0] != rows[0] || got[1] != rows[1] {
		t.Errorf("got rows %+v, want %+v", got, rows)
	}
}

func TestWriteJSONLines_NilClientIsNoop(t *testing.T) {
	var c *Client
	if err := c.WriteJSONLines(context.Background(), "irrelevant", []sampleRow{{ID: "x"}}, time.Now()); err != nil {
		t.Errorf("nil client should no-op, got error: %v", err)
	}
}
