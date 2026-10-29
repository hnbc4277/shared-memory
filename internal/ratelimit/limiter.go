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
	// Limit is max requests per Window for authenticated hot paths. Zero disables limiting.
	Limit int
	// PublicLimit is for login/register by IP; zero falls back to Limit.
	PublicLimit int
	// EmailAuthLimit is for login by normalized email; zero disables.
	EmailAuthLimit int
	Window         time.Duration
}

func (l *Limiter) publicLimit() int {
	if l != nil && l.PublicLimit > 0 {
		return l.PublicLimit
	}
	if l == nil {
		return 0
	}
	return l.Limit
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

// AllowIP limits unauthenticated routes (login/register) by client IP.
func (l *Limiter) AllowIP(ctx context.Context, clientIP, route string) (bool, error) {
	lim := l.publicLimit()
	if l == nil || l.Redis == nil || lim <= 0 || clientIP == "" {
		return true, nil
	}
	if l.Window <= 0 {
		l.Window = time.Minute
	}
	key := fmt.Sprintf("sm:rl:ip:%s:%s:%d", route, clientIP, time.Now().UTC().Unix()/int64(l.Window.Seconds()))
	n, err := l.Redis.Incr(ctx, key).Result()
	if err != nil {
		return true, err
	}
	if n == 1 {
		_ = l.Redis.Expire(ctx, key, l.Window+time.Second).Err()
	}
	return n <= int64(lim), nil
}

// AllowEmail limits login attempts per normalized email (route e.g. auth_login_email).
func (l *Limiter) AllowEmail(ctx context.Context, email, route string) (bool, error) {
	if l == nil || l.Redis == nil || l.EmailAuthLimit <= 0 || email == "" {
		return true, nil
	}
	if l.Window <= 0 {
		l.Window = time.Minute
	}
	key := fmt.Sprintf("sm:rl:email:%s:%s:%d", route, email, time.Now().UTC().Unix()/int64(l.Window.Seconds()))
	n, err := l.Redis.Incr(ctx, key).Result()
	if err != nil {
		return true, err
	}
	if n == 1 {
		_ = l.Redis.Expire(ctx, key, l.Window+time.Second).Err()
	}
	return n <= int64(l.EmailAuthLimit), nil
}
