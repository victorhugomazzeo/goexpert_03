package limiter

import (
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	rules := Rules{
		IP:     Limit{MaxRequests: 2, BlockDuration: 5 * time.Minute},
		Tokens: map[string]Limit{"abc123": {MaxRequests: 100, BlockDuration: time.Minute}},
	}

	cases := []struct {
		name      string
		req       Request
		wantKey   string
		wantLimit Limit
		wantScope string
	}{
		{
			name:      "no token falls back to the IP limit",
			req:       Request{IP: "1.1.1.1"},
			wantKey:   "ip:1.1.1.1",
			wantLimit: rules.IP,
			wantScope: "ip",
		},
		{
			name:      "configured token overrides the IP limit",
			req:       Request{IP: "1.1.1.1", Token: "abc123"},
			wantKey:   "token:abc123",
			wantLimit: rules.Tokens["abc123"],
			wantScope: "token",
		},
		{
			name:      "unknown token falls back to the IP limit",
			req:       Request{IP: "1.1.1.1", Token: "f4c1-random-garbage"},
			wantKey:   "ip:1.1.1.1",
			wantLimit: rules.IP,
			wantScope: "ip",
		},
		{
			name:      "token shaped like an IP does not poison the IP counter",
			req:       Request{IP: "9.9.9.9", Token: "1.1.1.1"},
			wantKey:   "ip:9.9.9.9",
			wantLimit: rules.IP,
			wantScope: "ip",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			key, limit, scope := rules.resolve(tt.req)

			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
			if limit != tt.wantLimit {
				t.Errorf("limit = %+v, want %+v", limit, tt.wantLimit)
			}
			if scope != tt.wantScope {
				t.Errorf("scope = %q, want %q", scope, tt.wantScope)
			}
		})
	}
}
