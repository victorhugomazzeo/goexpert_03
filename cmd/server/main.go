package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/victorhugomazzeo/goexpert_03/internal/config"
	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
	"github.com/victorhugomazzeo/goexpert_03/internal/middleware"
	"github.com/victorhugomazzeo/goexpert_03/internal/strategy/memory"
	"github.com/victorhugomazzeo/goexpert_03/internal/strategy/redis"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx := context.Background()

	strategy, err := newStrategy(ctx, cfg)
	if err != nil {
		return err
	}
	defer strategy.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Healthy"))
	})

	lim := limiter.New(strategy, cfg.Rules(), cfg.Window)

	handler := middleware.New(lim)(mux)

	addr := ":" + cfg.ServerPort
	log.Printf("ouvindo em %s (strategy: %s)", addr, cfg.Strategy)

	if err := http.ListenAndServe(addr, handler); err != nil {
		return fmt.Errorf("servidor: %w", err)
	}
	return nil
}

func newStrategy(ctx context.Context, cfg *config.Config) (limiter.Strategy, error) {
	switch cfg.Strategy {
	case "redis":
		return redis.New(ctx, redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
	case "memory":
		return memory.New(), nil
	default:
		return nil, fmt.Errorf("invalid strategy: %s", cfg.Strategy)
	}
}
