// Package strategytest runs a conformance suite against any implementation of
// limiter.Strategy. Running the same suite against every implementation is what
// proves they are interchangeable.
package strategytest

import (
	"context"
	"testing"
	"time"

	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
)

const (
	window = 200 * time.Millisecond
	block  = 600 * time.Millisecond
)

// Run executes the suite.
//
//	newStore: returns a clean store, ready to use.
//	advance:  moves time forward (fake clock in memory, time.Sleep in Redis).
func Run(t *testing.T, newStore func() limiter.Strategy, advance func(time.Duration)) {
	t.Helper()
	ctx := context.Background()
	lim := limiter.Limit{MaxRequests: 3, BlockDuration: block}

	t.Run("allows up to the limit then denies", func(t *testing.T) {
		st := newStore()
		defer st.Close()

		for i := 1; i <= 3; i++ {
			s, err := st.Check(ctx, "k1", lim, window)
			if err != nil {
				t.Fatalf("request %d: %v", i, err)
			}
			if !s.Allowed {
				t.Fatalf("request %d: denied, limit is 3 (Hits=%d)", i, s.Hits)
			}
		}

		s, _ := st.Check(ctx, "k1", lim, window)
		if s.Allowed {
			t.Error("request 4: allowed, should have exceeded the limit")
		}
		if s.Blocked {
			t.Error("request 4: Blocked = true, but this is the request that exceeds now")
		}

		s, _ = st.Check(ctx, "k1", lim, window)
		if s.Allowed {
			t.Error("request 5: allowed, should be blocked")
		}
		if !s.Blocked {
			t.Error("request 5: Blocked = false, but it was already serving a block")
		}
	})

	t.Run("window resets", func(t *testing.T) {
		st := newStore()
		defer st.Close()

		for i := 1; i <= 3; i++ {
			st.Check(ctx, "k2", lim, window)
		}
		advance(window)

		s, err := st.Check(ctx, "k2", lim, window)
		if err != nil {
			t.Fatal(err)
		}
		if !s.Allowed {
			t.Error("should be allowed after the window expires")
		}
		if s.Hits != 1 {
			t.Errorf("Hits = %d, want 1 (counter resets in a new window)", s.Hits)
		}
	})

	t.Run("block persists beyond the window", func(t *testing.T) {
		st := newStore()
		defer st.Close()

		for i := 1; i <= 4; i++ {
			st.Check(ctx, "k3", lim, window) // the 4th triggers the block
		}

		advance(window + 50*time.Millisecond) // window over, block still active
		s, _ := st.Check(ctx, "k3", lim, window)
		if s.Allowed {
			t.Error("window expired but the block is active: should still deny")
		}
		if !s.Blocked {
			t.Error("Blocked = false, but the key is serving a block")
		}
		if s.RetryAfter <= 0 {
			t.Errorf("RetryAfter = %v, want > 0", s.RetryAfter)
		}

		advance(block) // block served
		s, _ = st.Check(ctx, "k3", lim, window)
		if !s.Allowed {
			t.Error("block served: should be allowed")
		}
		if s.Hits != 1 {
			t.Errorf("Hits = %d, want 1", s.Hits)
		}
	})

	t.Run("keys are independent", func(t *testing.T) {
		st := newStore()
		defer st.Close()

		for i := 1; i <= 4; i++ {
			st.Check(ctx, "ip:1.1.1.1", lim, window) // exceed this one
		}

		s, err := st.Check(ctx, "ip:2.2.2.2", lim, window)
		if err != nil {
			t.Fatal(err)
		}
		if !s.Allowed {
			t.Error("a different key must not be affected")
		}
	})

	t.Run("Reset clears the key", func(t *testing.T) {
		st := newStore()
		defer st.Close()

		for i := 1; i <= 4; i++ {
			st.Check(ctx, "k5", lim, window)
		}
		if err := st.Reset(ctx, "k5"); err != nil {
			t.Fatalf("Reset: %v", err)
		}

		s, _ := st.Check(ctx, "k5", lim, window)
		if !s.Allowed {
			t.Error("should be allowed after Reset")
		}
		if s.Hits != 1 {
			t.Errorf("Hits = %d, want 1", s.Hits)
		}
	})
}
