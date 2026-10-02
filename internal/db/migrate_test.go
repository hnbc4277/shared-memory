//go:build integration

package db_test

import (
	"context"
	"os"
	"testing"

	"sharedmemory/internal/db"
)

func TestMigrate_CreatesAccountsTable(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://sm:sm@127.0.0.1:15432/sharedmemory?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_name='accounts'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("accounts table missing")
	}
}
