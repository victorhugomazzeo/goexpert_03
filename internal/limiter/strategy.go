package limiter

import (
	"context"
	"time"
)

type Limit struct {
	MaxRequests   int
	BlockDuration time.Duration
}

type Rules struct {
	IP     Limit
	Tokens map[string]Limit
}

type State struct {
	Allowed    bool
	Blocked    bool
	Hits       int64
	RetryAfter time.Duration
}

type Strategy interface {
	Check(ctx context.Context, key string, limit Limit, window time.Duration) (State, error)
	Reset(ctx context.Context, key string) error
	Close() error
}
