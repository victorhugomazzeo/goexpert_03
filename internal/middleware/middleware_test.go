package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
	"github.com/victorhugomazzeo/goexpert_03/internal/middleware"
	"github.com/victorhugomazzeo/goexpert_03/internal/strategy/memory"
)

// newHandler wires the chain: middleware -> final handler that writes "ok".
// If the response body is "ok" the final handler ran; if it is the limit
// message, the middleware stopped the request before it.
func newHandler(t *testing.T, rules limiter.Rules) http.Handler {
	t.Helper()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := memory.NewWithClock(func() time.Time { return now })
	t.Cleanup(func() { _ = store.Close() })

	l := limiter.New(store, rules, time.Second)

	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return middleware.New(l)(final)
}

// doRequest fires a fake request and returns the recorded response.
func doRequest(h http.Handler, ip, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = ip + ":54321"
	if token != "" {
		req.Header.Set("API_KEY", token)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAllowsBelowTheLimit(t *testing.T) {
	h := newHandler(t, limiter.Rules{
		IP: limiter.Limit{MaxRequests: 2, BlockDuration: time.Minute},
	})

	rec := doRequest(h, "1.1.1.1", "")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q (the final handler should have run)", rec.Body.String(), "ok")
	}
}

func TestReturns429WithExactBody(t *testing.T) {
	h := newHandler(t, limiter.Rules{
		IP: limiter.Limit{MaxRequests: 1, BlockDuration: 5 * time.Minute},
	})

	doRequest(h, "1.1.1.1", "") // consumes the limit
	rec := doRequest(h, "1.1.1.1", "")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Body.String(); got != middleware.MessageLimitReached {
		t.Errorf("wrong body\n got  = %q\n want = %q", got, middleware.MessageLimitReached)
	}

	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("Retry-After header is missing")
	}
	if _, err := strconv.Atoi(retryAfter); err != nil {
		t.Errorf("Retry-After = %q, want an integer number of seconds", retryAfter)
	}
}

func TestAPIKeyHeaderOverridesIP(t *testing.T) {
	h := newHandler(t, limiter.Rules{
		IP:     limiter.Limit{MaxRequests: 1, BlockDuration: 5 * time.Minute},
		Tokens: map[string]limiter.Limit{"abc123": {MaxRequests: 5, BlockDuration: time.Minute}},
	})

	for i := 1; i <= 5; i++ {
		rec := doRequest(h, "1.1.1.1", "abc123")
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d with API_KEY: status = %d, want 200 (token limit is 5)", i, rec.Code)
		}
	}

	if rec := doRequest(h, "1.1.1.1", "abc123"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("request 6: status = %d, want 429", rec.Code)
	}
}

func TestDifferentIPsHaveIndependentCounters(t *testing.T) {
	h := newHandler(t, limiter.Rules{
		IP: limiter.Limit{MaxRequests: 1, BlockDuration: 5 * time.Minute},
	})

	doRequest(h, "1.1.1.1", "")
	if rec := doRequest(h, "1.1.1.1", ""); rec.Code != http.StatusTooManyRequests {
		t.Errorf("1.1.1.1 second request: status = %d, want 429", rec.Code)
	}
	if rec := doRequest(h, "2.2.2.2", ""); rec.Code != http.StatusOK {
		t.Errorf("2.2.2.2 first request: status = %d, want 200", rec.Code)
	}
}
