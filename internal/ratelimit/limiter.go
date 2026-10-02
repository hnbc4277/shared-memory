// Package ratelimit provides a simple Redis fixed-window limiter for API hot paths.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	Redis *redis.Client
	// Limit is max requests per Window. Zero disables limiting.
	Limit  int
	Window time.Duration
}

func (l *Limiter) Allow(ctx context.Context, accountID uuid.UUID, route string) (bool, error) {
	if l == nil || l.Redis == nil || l.Limit <= 0 {
		return true, nil
	}
	if l.Window <= 0 {
		l.Window = time.Minute
	}
	key := fmt.Sprintf("sm:rl:%s:%s:%d", accountID, route, time.Now().UTC().Unix()/int64(l.Window.Seconds()))
	n, err := l.Redis.Incr(ctx, key).Result()
	if err != nil {
		// Fail open when Redis is down so evidence writes are not blocked.
		return true, err
	}
	if n == 1 {
		_ = l.Redis.Expire(ctx, key, l.Window+time.Second).Err()
	}
	return n <= int64(l.Limit), nil
}
