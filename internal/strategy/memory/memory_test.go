package memory_test

import (
	"testing"
	"time"

	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
	"github.com/victorhugomazzeo/goexpert_03/internal/strategy/memory"
	"github.com/victorhugomazzeo/goexpert_03/internal/strategy/strategytest"
)

func TestMemoryStrategy(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base

	strategytest.Run(t,
		func() limiter.Strategy {
			now = base // rewind the clock for each subtest
			return memory.NewWithClock(func() time.Time { return now })
		},
		func(d time.Duration) { now = now.Add(d) },
	)
}
