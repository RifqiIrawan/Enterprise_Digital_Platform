// Package datalake adalah landing zone mentah (bronze layer) di MinIO --
// SETIAP fact yang di-sync ke ClickHouse (curated/query layer) juga ditulis
// sebagai file JSON Lines di sini, dalam siklus sync yang SAMA. Ini
// pengecualian dari desain "1 file per sync", bukan sumber data independen:
// data lake ini SELALU rebuild-able dari sumber Postgres yang sama seperti
// fact ClickHouse-nya, jadi tetap aman kalau file di sini hilang/korup.
//
// Ditulis best-effort, sama seperti publish Kafka di service lain di
// project ini (Client bisa nil -- caller cukup cek nil sebelum menulis,
// kegagalan tulis TIDAK boleh menggagalkan sync ke ClickHouse yang tetap
// jadi destinasi utama/curated).
package datalake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc     *minio.Client
	bucket string
}

// Connect membuka koneksi ke MinIO (atau S3-compatible lain) dan memastikan
// bucket tujuan ada (create kalau belum, idempotent -- pola sama dengan
// clickhouse.Connect's CREATE DATABASE IF NOT EXISTS).
func Connect(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("open minio client: %w", err)
	}

	exists, err := mc.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket %s: %w", bucket, err)
	}
	if !exists {
		if err := mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket %s: %w", bucket, err)
		}
	}

	return &Client{mc: mc, bucket: bucket}, nil
}

// WriteJSONLines mengubah rows (slice bertipe apa pun, biasanya []ch.XRow
// dari package clickhouse) jadi satu file JSON Lines (satu objek JSON per
// baris, encoding/json default -- nama field Go apa adanya, BUKAN
// snake_case seperti kolom ClickHouse, karena ini raw/bronze layer yang
// tujuannya durability & reprocessability, bukan konsumsi langsung) lalu
// upload ke <bucket>/<fact>/<YYYY>/<MM>/<DD>/<unix-nano>.jsonl. Key
// mengandung syncedAt (bukan waktu upload) supaya reproducible kalau
// dipanggil ulang dengan data historis yang sama.
func (c *Client) WriteJSONLines(ctx context.Context, fact string, rows any, syncedAt time.Time) error {
	if c == nil {
		return nil
	}

	// rows adalah slice -- json.Marshal per elemen (bukan Marshal seluruh
	// slice sekaligus) supaya hasilnya benar-benar JSON *Lines* (satu objek
	// per baris, format yang bisa di-stream-parse baris demi baris), bukan
	// satu array JSON besar.
	v, err := toAnySlice(rows)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	for _, row := range v {
		b, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("marshal row for %s: %w", fact, err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}

	key := fmt.Sprintf("%s/%04d/%02d/%02d/%d.jsonl", fact, syncedAt.Year(), syncedAt.Month(), syncedAt.Day(), syncedAt.UnixNano())
	_, err = c.mc.PutObject(ctx, c.bucket, key, &buf, int64(buf.Len()), minio.PutObjectOptions{ContentType: "application/x-ndjson"})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

// ListKeys mengembalikan semua object key di bawah prefix -- dipakai test
// untuk memverifikasi WriteJSONLines benar-benar menulis ke MinIO, bukan
// API produksi.
func (c *Client) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	for obj := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list objects under %s: %w", prefix, obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}

// Get mengambil isi mentah sebuah object -- dipakai test untuk baca balik
// file JSONL yang ditulis WriteJSONLines dan verifikasi isinya.
func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}
	defer obj.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(obj); err != nil {
		return nil, fmt.Errorf("read object %s: %w", key, err)
	}
	return buf.Bytes(), nil
}

// toAnySlice mengubah slice bertipe konkret ([]ch.FinanceJournalLineRow,
// dst) jadi []any lewat reflection, supaya WriteJSONLines bisa dipakai satu
// method untuk semua 9 fact tanpa generic per tipe. reflect dipakai sengaja
// di sini (bukan pola umum di codebase ini) karena signature caller-nya
// (etl.SyncX) sudah bertipe konkret per fact -- pilihannya cuma reflection
// di sini SATU KALI, atau 9 varian WriteJSONLines* yang isinya identik.
func toAnySlice(rows any) ([]any, error) {
	rv := reflect.ValueOf(rows)
	if rv.Kind() != reflect.Slice {
		return nil, fmt.Errorf("datalake: rows must be a slice, got %T", rows)
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, nil
}
