package datalake

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/minio/minio-go/v7"
)

// Silver adalah lapisan kedua data lake: keadaan TERKINI setiap baris fact,
// sudah dibersihkan dari duplikat. Bronze append-only -- baris yang berubah
// (atau yang ditulis dua kali oleh batch ETL dan Kafka streaming ETL) muncul
// di banyak file, dan membaca Bronze mentah-mentah menghitungnya berkali-kali.
//
// Aturannya SAMA dengan ReplacingMergeTree(synced_at) di ClickHouse: kunci
// (CompanyID, id-baris), versi terbaru menang. Bronze dibaca berurutan
// menurut key object (tanggal lalu unix-nano synced_at), jadi "terbaru" di
// sini berarti synced_at terbesar -- keduanya sengaja tidak boleh berbeda
// pendapat tentang baris mana yang benar (lihat silver_test.go).
//
// Baris tanpa kunci, dengan kunci UUID kosong, atau bukan JSON tidak
// dibuang diam-diam: dihitung, dan disimpan di rejected.jsonl supaya bisa
// diperiksa. Kalau tidak ada yang ditolak, berkas lama dihapus supaya tidak
// ada "penolakan" basi yang tersisa dari build sebelumnya.

const silverPrefix = "silver/"

const zeroUUID = "00000000-0000-0000-0000-000000000000"

// SilverFacts memetakan nama fact (prefix di Bronze) ke nama field Go yang
// jadi id barisnya. Nama field, bukan nama kolom: Bronze menulis struct Go
// apa adanya lewat encoding/json (lihat WriteJSONLines). Kalau fact baru
// ditambahkan ke ETL tapi lupa didaftarkan di sini, TestSilverFactsCoverEveryFact
// gagal.
var SilverFacts = map[string]string{
	"finance_journal_lines":  "LineID",
	"sales_order_lines":      "LineID",
	"inventory_movements":    "MovementID",
	"hr_payroll_details":     "DetailID",
	"hr_leave_requests":      "LeaveID",
	"hr_kpi_reviews":         "ReviewID",
	"purchasing_order_lines": "LineID",
	"production_work_orders": "WOID",
	"production_oee":         "RunID",
	"qc_inspections":         "InspectionID",
	"asset_maintenance":      "ScheduleID",
	"iot_readings":           "ReadingID",
	"opportunities":          "OpportunityID",
	"tickets":                "TicketID",
	"delivery_orders":        "DeliveryID",
	"timesheets":             "TimesheetID",
	"order_items":            "LineID",
}

// SilverStats meringkas satu build Silver. BronzeRows = SilverRows +
// DuplicatesDropped + Rejected selalu berlaku; itu invarian yang diuji.
type SilverStats struct {
	Fact              string `json:"fact"`
	BronzeObjects     int    `json:"bronze_objects"`
	BronzeRows        int    `json:"bronze_rows"`
	SilverRows        int    `json:"silver_rows"`
	DuplicatesDropped int    `json:"duplicates_dropped"`
	Rejected          int    `json:"rejected"`
}

func silverKey(fact string) string       { return silverPrefix + fact + "/current.jsonl" }
func silverRejectKey(fact string) string { return silverPrefix + fact + "/rejected.jsonl" }

// BuildSilver membangun ulang Silver satu fact dari SELURUH Bronze-nya. Ini
// full rebuild, bukan incremental: Bronze cukup kecil untuk dibaca ulang,
// dan hasilnya jadi deterministik dan idempotent -- dua build berturut-turut
// tanpa data baru menghasilkan berkas yang byte-per-byte sama. Seluruh
// fact ditampung di memori (map kunci -> baris); kalau Bronze tumbuh sampai
// itu jadi masalah, itulah saatnya berpindah ke Spark/dbt.
func (c *Client) BuildSilver(ctx context.Context, fact string) (SilverStats, error) {
	idField, ok := SilverFacts[fact]
	if !ok {
		return SilverStats{Fact: fact}, fmt.Errorf("datalake: fact %q tidak dikenal", fact)
	}
	return c.buildSilver(ctx, fact, idField)
}

// buildSilver adalah BuildSilver dengan field id yang sudah ditentukan;
// dipisah supaya test bisa memakai nama fact sekali-pakai tanpa menyentuh
// Bronze fact sungguhan di bucket yang sama.
func (c *Client) buildSilver(ctx context.Context, fact, idField string) (SilverStats, error) {
	stats := SilverStats{Fact: fact}
	if c == nil {
		return stats, fmt.Errorf("datalake: data lake tidak tersedia")
	}

	keys, err := c.ListKeys(ctx, fact+"/")
	if err != nil {
		return stats, err
	}

	latest := map[string][]byte{}
	var rejected [][]byte
	for _, k := range keys {
		data, err := c.Get(ctx, k)
		if err != nil {
			return stats, err
		}
		stats.BronzeObjects++

		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			stats.BronzeRows++

			var probe map[string]json.RawMessage
			if err := json.Unmarshal(line, &probe); err != nil {
				rejected = append(rejected, append([]byte(nil), line...))
				continue
			}
			rowID := unquote(probe[idField])
			companyID := unquote(probe["CompanyID"])
			if !validID(rowID) || !validID(companyID) {
				rejected = append(rejected, append([]byte(nil), line...))
				continue
			}
			// Bronze dibaca dari yang terlama ke terbaru, jadi menimpa = versi
			// terbaru menang.
			latest[companyID+"/"+rowID] = append([]byte(nil), line...)
		}
		if err := sc.Err(); err != nil {
			return stats, fmt.Errorf("scan %s: %w", k, err)
		}
	}

	ordered := make([]string, 0, len(latest))
	for k := range latest {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)

	var out bytes.Buffer
	for _, k := range ordered {
		out.Write(latest[k])
		out.WriteByte('\n')
	}
	stats.SilverRows = len(latest)
	stats.Rejected = len(rejected)
	stats.DuplicatesDropped = stats.BronzeRows - stats.SilverRows - stats.Rejected

	if err := c.put(ctx, silverKey(fact), out.Bytes()); err != nil {
		return stats, err
	}
	if len(rejected) > 0 {
		var rb bytes.Buffer
		for _, l := range rejected {
			rb.Write(l)
			rb.WriteByte('\n')
		}
		if err := c.put(ctx, silverRejectKey(fact), rb.Bytes()); err != nil {
			return stats, err
		}
	} else if err := c.mc.RemoveObject(ctx, c.bucket, silverRejectKey(fact), minio.RemoveObjectOptions{}); err != nil {
		return stats, fmt.Errorf("remove stale %s: %w", silverRejectKey(fact), err)
	}
	return stats, nil
}

// ReadSilver mengembalikan baris-baris Silver satu fact (satu JSON per
// elemen). Fact yang belum pernah di-build mengembalikan slice kosong, bukan
// galat -- "belum ada" dan "kosong" sama artinya bagi pemanggil (Gold).
func (c *Client) ReadSilver(ctx context.Context, fact string) ([][]byte, error) {
	return c.readLines(ctx, silverKey(fact))
}

func (c *Client) readLines(ctx context.Context, key string) ([][]byte, error) {
	if c == nil {
		return nil, nil
	}
	if _, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{}); err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", key, err)
	}
	data, err := c.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	var out [][]byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		if l := bytes.TrimSpace(sc.Bytes()); len(l) > 0 {
			out = append(out, append([]byte(nil), l...))
		}
	}
	return out, sc.Err()
}

func (c *Client) put(ctx context.Context, key string, data []byte) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/x-ndjson"})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

// Put dan Remove dipakai test untuk menyiapkan/membersihkan objek; API
// produksi cukup BuildSilver/BuildGold.
func (c *Client) Put(ctx context.Context, key string, data []byte) error {
	return c.put(ctx, key, data)
}

func (c *Client) Remove(ctx context.Context, key string) error {
	return c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})
}

func unquote(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func validID(s string) bool {
	return strings.TrimSpace(s) != "" && s != zeroUUID
}
