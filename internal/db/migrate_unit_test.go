package db_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"sharedmemory/internal/db"
)

func TestConnect_RejectsEmptyDSN(t *testing.T) {
	_, err := db.Connect(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

func TestMigrate_MissingDir(t *testing.T) {
	err := db.Migrate(context.Background(), nil, t.TempDir()+"/no-such-migrations")
	if err == nil {
		t.Fatal("expected error for missing migrations dir")
	}
}

func TestInitSQL_DeclaresRequiredTables(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/0001_init.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(raw))
	required := []string{
		"accounts",
		"api_keys",
		"projects",
		"account_policies",
		"sessions",
		"messages",
		"events",
		"attachments",
		"memories",
		"profiles",
		"audit_logs",
	}
	for _, table := range required {
		if !strings.Contains(body, "create table") || !strings.Contains(body, table) {
			t.Fatalf("0001_init.up.sql must create table %s", table)
		}
	}
}
