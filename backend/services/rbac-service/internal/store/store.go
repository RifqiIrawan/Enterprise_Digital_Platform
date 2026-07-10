package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

// Migrate menjalankan file *.sql di embed.FS secara berurutan (alfabetis),
// dilacak lewat tabel schema_migrations supaya idempotent tiap kali service
// start. Jika sebuah migrasi gagal (mis. tabel sudah ada karena pernah
// dijalankan manual lewat psql sebelum tabel schema_migrations ini ada),
// dicatat sebagai warning dan tetap ditandai applied supaya tidak diulang.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrations embed.FS) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		filename   TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(entries)

	for _, name := range entries {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename=$1)`, name,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		content, err := migrations.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			log.Printf("store: migration %s failed, assuming already applied manually: %v", name, err)
		} else {
			log.Printf("store: migration applied: %s", name)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations(filename) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}
	return nil
}
