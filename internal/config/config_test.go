package config

import (
	"testing"
	"time"
)

func TestParseTokenLimits(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantLen int
		wantErr bool
	}{
		{"empty string yields an empty map", "", 0, false},
		{"single token", "abc123:100:5m", 1, false},
		{"multiple tokens", "abc:1:5m,def:2:1m,ghi:3:30s", 3, false},
		{"space after the comma", "abc:1:5m, def:2:1m", 2, false},
		{"trailing comma", "abc:1:5m,", 1, false},
		{"too few fields", "abc:100", 0, true},
		{"too many fields", "abc:100:5m:xx", 0, true},
		{"non numeric limit", "abc:xyz:5m", 0, true},
		{"invalid duration", "abc:100:5x", 0, true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTokenLimits(tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("want an error, got nil (result = %v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d (result = %v)", len(got), tt.wantLen, got)
			}
		})
	}
}

func TestParseTokenLimitsValues(t *testing.T) {
	got, err := parseTokenLimits("abc123:100:5m")
	if err != nil {
		t.Fatal(err)
	}

	lim, ok := got["abc123"]
	if !ok {
		t.Fatalf("token abc123 not found in %v", got)
	}
	if lim.MaxRequests != 100 {
		t.Errorf("MaxRequests = %d, want 100", lim.MaxRequests)
	}
	if lim.BlockDuration != 5*time.Minute {
		t.Errorf("BlockDuration = %v, want 5m", lim.BlockDuration)
	}
}

func TestLoadUsesDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ServerPort != "8080" {
		t.Errorf("ServerPort = %q, want %q", cfg.ServerPort, "8080")
	}
	if cfg.Strategy != "redis" {
		t.Errorf("Strategy = %q, want %q", cfg.Strategy, "redis")
	}
	if cfg.Window != time.Second {
		t.Errorf("Window = %v, want 1s", cfg.Window)
	}
	if cfg.IPMaxRequests != 10 {
		t.Errorf("IPMaxRequests = %d, want 10", cfg.IPMaxRequests)
	}
	if cfg.IPBlockDuration != 5*time.Minute {
		t.Errorf("IPBlockDuration = %v, want 5m", cfg.IPBlockDuration)
	}
	if len(cfg.Tokens) != 0 {
		t.Errorf("Tokens = %v, want empty", cfg.Tokens)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	clearEnv(t)
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("RATE_LIMITER_STRATEGY", "memory")
	t.Setenv("RATE_LIMIT_IP_MAX_REQUESTS", "3")
	t.Setenv("RATE_LIMIT_IP_BLOCK_DURATION", "30s")
	t.Setenv("RATE_LIMIT_TOKENS", "abc123:100:5m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ServerPort != "9090" {
		t.Errorf("ServerPort = %q, want %q", cfg.ServerPort, "9090")
	}
	if cfg.Strategy != "memory" {
		t.Errorf("Strategy = %q, want %q", cfg.Strategy, "memory")
	}
	if cfg.IPMaxRequests != 3 {
		t.Errorf("IPMaxRequests = %d, want 3", cfg.IPMaxRequests)
	}
	if cfg.IPBlockDuration != 30*time.Second {
		t.Errorf("IPBlockDuration = %v, want 30s", cfg.IPBlockDuration)
	}

	rules := cfg.Rules()
	if rules.IP.MaxRequests != 3 {
		t.Errorf("Rules().IP.MaxRequests = %d, want 3", rules.IP.MaxRequests)
	}
	if rules.Tokens["abc123"].MaxRequests != 100 {
		t.Errorf("Rules().Tokens[abc123] = %+v, want MaxRequests 100", rules.Tokens["abc123"])
	}
}

func TestLoadFailsOnInvalidValue(t *testing.T) {
	clearEnv(t)
	t.Setenv("RATE_LIMIT_IP_BLOCK_DURATION", "five minutes")

	if _, err := Load(); err == nil {
		t.Error("want an error for an invalid duration, got nil")
	}
}

// clearEnv unsets the rate limiter variables so the tests do not depend on the
// environment of whoever runs them. t.Setenv restores everything afterwards.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SERVER_PORT", "RATE_LIMITER_STRATEGY",
		"REDIS_ADDR", "REDIS_PASSWORD", "REDIS_DB",
		"RATE_LIMIT_WINDOW", "RATE_LIMIT_IP_MAX_REQUESTS",
		"RATE_LIMIT_IP_BLOCK_DURATION", "RATE_LIMIT_TOKENS",
	} {
		t.Setenv(k, "")
	}
}
