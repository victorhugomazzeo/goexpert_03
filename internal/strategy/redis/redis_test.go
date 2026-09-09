package redis_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
	redisstore "github.com/victorhugomazzeo/goexpert_03/internal/strategy/redis"
	"github.com/victorhugomazzeo/goexpert_03/internal/strategy/strategytest"
)

// TestRedisStrategy roda a mesma suite de conformidade da strategy de
// memoria. As duas passando na mesma suite e o que prova que sao
// intercambiaveis.
//
// Aqui advance e time.Sleep de verdade: nao ha relogio para injetar, os
// prazos sao os TTLs do Redis. A suite trabalha com 200ms e 600ms, entao o
// teste inteiro leva cerca de dois segundos.
func TestRedisStrategy(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}

	ctx := context.Background()

	probe, err := redisstore.New(ctx, redisstore.Options{Addr: addr})
	if err != nil {
		t.Skipf("redis indisponivel em %s: %v (suba com: docker compose up -d)", addr, err)
	}
	_ = probe.Close()

	// Prefixo proprio deste processo. Isola a corrida sem FLUSHDB, que
	// apagaria o que mais estiver no Redis de quem roda o teste.
	prefix := fmt.Sprintf("ratelimit-test:%d", os.Getpid())

	strategytest.Run(t,
		func() limiter.Strategy {
			st, err := redisstore.New(ctx, redisstore.Options{Addr: addr, Prefix: prefix})
			if err != nil {
				t.Fatalf("nova store: %v", err)
			}
			return st
		},
		time.Sleep,
	)
}
