package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
)

type Config struct {
	ServerPort string
	Strategy   string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	Window          time.Duration
	IPMaxRequests   int
	IPBlockDuration time.Duration
	Tokens          map[string]limiter.Limit
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	ipMax, err := getenvInt("RATE_LIMIT_IP_MAX_REQUESTS", 10)
	if err != nil {
		return nil, err
	}
	ipBlock, err := getenvDuration("RATE_LIMIT_IP_BLOCK_DURATION", 5*time.Minute)
	if err != nil {
		return nil, err
	}
	window, err := getenvDuration("RATE_LIMIT_WINDOW", time.Second)
	if err != nil {
		return nil, err
	}
	redisDB, err := getenvInt("REDIS_DB", 0)
	if err != nil {
		return nil, err
	}
	tokens, err := parseTokenLimits(getenv("RATE_LIMIT_TOKENS", ""))
	if err != nil {
		return nil, err
	}

	return &Config{
		ServerPort:      getenv("SERVER_PORT", "8080"),
		Strategy:        getenv("RATE_LIMITER_STRATEGY", "redis"),
		RedisAddr:       getenv("REDIS_ADDR", "redis:6379"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		RedisDB:         redisDB,
		Window:          window,
		IPMaxRequests:   ipMax,
		IPBlockDuration: ipBlock,
		Tokens:          tokens,
	}, nil
}

func parseTokenLimits(raw string) (map[string]limiter.Limit, error) {
	tokens := map[string]limiter.Limit{}

	for token := range strings.SplitSeq(raw, ",") {

		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		parts := strings.Split(token, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid token format: %s", token)
		}

		token := parts[0]

		maxRequests, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid max requests for token %s: %v", token, err)
		}

		blockDuration, err := time.ParseDuration(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid block duration for token %s: %v", token, err)
		}

		tokens[token] = limiter.Limit{
			MaxRequests:   maxRequests,
			BlockDuration: blockDuration,
		}
	}

	return tokens, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func getenvDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func (c *Config) Rules() limiter.Rules {
	return limiter.Rules{
		IP:     limiter.Limit{MaxRequests: c.IPMaxRequests, BlockDuration: c.IPBlockDuration},
		Tokens: c.Tokens,
	}
}
