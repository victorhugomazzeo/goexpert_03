package limiter_test

import (
	"context"
	"testing"
	"time"

	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
	"github.com/victorhugomazzeo/goexpert_03/internal/strategy/memory"
)

// newLimiter builds a limiter backed by the in-memory store with a controlled
// clock. It returns the limiter and a function that advances time.
func newLimiter(t *testing.T, rules limiter.Rules) (*limiter.Limiter, func(time.Duration)) {
	t.Helper()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := memory.NewWithClock(func() time.Time { return now })
	t.Cleanup(func() { _ = store.Close() })

	return limiter.New(store, rules, time.Second), func(d time.Duration) { now = now.Add(d) }
}

func TestAllowLimitsByIP(t *testing.T) {
	l, _ := newLimiter(t, limiter.Rules{
		IP: limiter.Limit{MaxRequests: 2, BlockDuration: 5 * time.Minute},
	})

	req := limiter.Request{IP: "1.1.1.1"}

	for i, want := range []bool{true, true, false, false} {
		d, err := l.Allow(context.Background(), req)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		if d.Allowed != want {
			t.Errorf("request %d: Allowed = %v, want %v (Hits=%d)", i+1, d.Allowed, want, d.Hits)
		}
	}
}

func TestAllowTokenOverridesIP(t *testing.T) {
	l, _ := newLimiter(t, limiter.Rules{
		IP:     limiter.Limit{MaxRequests: 2, BlockDuration: 5 * time.Minute},
		Tokens: map[string]limiter.Limit{"abc123": {MaxRequests: 5, BlockDuration: time.Minute}},
	})
	ctx := context.Background()

	// Five requests carrying the token all pass, even though the IP limit is 2.
	withToken := limiter.Request{IP: "1.1.1.1", Token: "abc123"}
	for i := 1; i <= 5; i++ {
		d, err := l.Allow(ctx, withToken)
		if err != nil {
			t.Fatalf("token request %d: %v", i, err)
		}
		if !d.Allowed {
			t.Fatalf("token request %d: denied, but the token limit is 5 (Hits=%d)", i, d.Hits)
		}
		if d.Scope != "token" {
			t.Errorf("token request %d: Scope = %q, want %q", i, d.Scope, "token")
		}
	}

	// The sixth exceeds the limit of the token itself.
	if d, _ := l.Allow(ctx, withToken); d.Allowed {
		t.Error("token request 6: allowed, but the token limit is 5")
	}

	// The IP counter is independent: three requests without a token, the third is denied.
	withoutToken := limiter.Request{IP: "1.1.1.1"}
	for i, want := range []bool{true, true, false} {
		d, err := l.Allow(ctx, withoutToken)
		if err != nil {
			t.Fatalf("ip request %d: %v", i+1, err)
		}
		if d.Allowed != want {
			t.Errorf("ip request %d: Allowed = %v, want %v", i+1, d.Allowed, want)
		}
	}
}

func TestAllowTokenWithLowerLimitThanIP(t *testing.T) {
	l, _ := newLimiter(t, limiter.Rules{
		IP:     limiter.Limit{MaxRequests: 100, BlockDuration: 5 * time.Minute},
		Tokens: map[string]limiter.Limit{"weak": {MaxRequests: 1, BlockDuration: 30 * time.Second}},
	})
	ctx := context.Background()
	req := limiter.Request{IP: "1.1.1.1", Token: "weak"}

	for i, want := range []bool{true, false} {
		d, err := l.Allow(ctx, req)
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		if d.Allowed != want {
			t.Errorf("request %d: Allowed = %v, want %v (token limit is 1, IP limit is 100)", i+1, d.Allowed, want)
		}
	}
}

func TestAllowBlockPersistsBeyondWindow(t *testing.T) {
	l, advance := newLimiter(t, limiter.Rules{
		IP: limiter.Limit{MaxRequests: 1, BlockDuration: 5 * time.Minute},
	})
	ctx := context.Background()
	req := limiter.Request{IP: "1.1.1.1"}

	if d, _ := l.Allow(ctx, req); !d.Allowed {
		t.Fatal("1st request should have been allowed")
	}

	d, _ := l.Allow(ctx, req)
	if d.Allowed {
		t.Fatal("2nd request should have exceeded the limit")
	}
	if d.Blocked {
		t.Error("2nd request: Blocked = true, but this is the request that exceeds now (want false)")
	}

	// The 1s window is over, the 5min block is not.
	advance(2 * time.Second)
	d, _ = l.Allow(ctx, req)
	if d.Allowed {
		t.Error("window expired but the block should still deny the request")
	}
	if !d.Blocked {
		t.Error("Blocked = false, but the IP is serving its block (want true)")
	}
	if d.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %v, want > 0", d.RetryAfter)
	}

	// Block served.
	advance(5 * time.Minute)
	d, _ = l.Allow(ctx, req)
	if !d.Allowed {
		t.Error("after the block expires the request should be allowed again")
	}
	if d.Hits != 1 {
		t.Errorf("Hits = %d, want 1 (counter resets when the block ends)", d.Hits)
	}
}
