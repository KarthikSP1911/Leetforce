package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testStore returns a Store on a throwaway schema built from the real
// migration files, dropped when the test ends, so tests never touch the real
// tables. It uses a direct (non-pooler) connection because a search_path set
// per connection is not reliable behind pgbouncer. Skips without a database.
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("LEETFORCE_MIGRATE_DATABASE_URL")
	if url == "" {
		url = strings.Replace(os.Getenv("DATABASE_URL"), "-pooler", "", 1)
	}
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	schema := "t_" + hex.EncodeToString(b)

	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 4
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO "+schema)
		return err
	}
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("database unreachable: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Skipf("cannot create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	// Never run migrations or tests against the real tables.
	var current string
	if err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&current); err != nil || current != schema {
		t.Fatalf("test pool is on schema %q (err %v), want %q; refusing to continue", current, err, schema)
	}

	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // repo file
		if err != nil {
			t.Fatal(err)
		}
		up := string(raw)
		if i := strings.Index(up, "-- +goose Down"); i >= 0 {
			up = up[:i]
		}
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	return &Store{pool: pool}
}
