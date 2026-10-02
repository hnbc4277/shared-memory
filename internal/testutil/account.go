package testutil

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/auth"
)

const TestPassword = "password123"

// NewAccount creates an account with a unique email so tests can rerun
// against the persistent compose database.
func NewAccount(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id, err := auth.NewStore(pool).CreateAccount(context.Background(), "t-"+uuid.NewString()+"@example.com", TestPassword)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

func Strs(v ...string) *[]string {
	if v == nil {
		v = []string{}
	}
	return &v
}
