package recall

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultRetentionDays = 30
	maxRetentionDays     = 3650
)

func normalizeRetentionDays(days int) int {
	if days <= 0 {
		return defaultRetentionDays
	}
	if days > maxRetentionDays {
		return maxRetentionDays
	}
	return days
}

// PurgeReceipts deletes recall_receipts older than retentionDays (default 30).
func PurgeReceipts(ctx context.Context, pool *pgxpool.Pool, retentionDays int) (int64, error) {
	if pool == nil {
		return 0, fmt.Errorf("recall: pool required")
	}
	days := normalizeRetentionDays(retentionDays)
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	tag, err := pool.Exec(ctx, `DELETE FROM recall_receipts WHERE queried_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
