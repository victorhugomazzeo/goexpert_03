package limiter

import (
	"context"
	"time"
)

type Limiter struct {
	strategy Strategy
	rules    Rules
	window   time.Duration
}

type Request struct {
	IP    string
	Token string
}

type Decision struct {
	Allowed    bool
	Blocked    bool
	Hits       int64
	RetryAfter time.Duration
	Scope      string
}

func New(strategy Strategy, rules Rules, window time.Duration) *Limiter {
	return &Limiter{
		strategy: strategy,
		rules:    rules,
		window:   window,
	}
}

func (l *Limiter) Allow(ctx context.Context, req Request) (Decision, error) {

	key, limit, scope := l.rules.resolve(req)

	st, err := l.strategy.Check(ctx, key, limit, l.window)
	if err != nil {
		return Decision{}, err
	}

	return Decision{
		Allowed:    st.Allowed,
		Blocked:    st.Blocked,
		Hits:       st.Hits,
		RetryAfter: st.RetryAfter,
		Scope:      scope,
	}, nil
}

func (r Rules) resolve(req Request) (string, Limit, string) {
	if req.Token != "" {
		if limit, ok := r.Tokens[req.Token]; ok {
			return "token:" + req.Token, limit, "token"
		}
	}
	return "ip:" + req.IP, r.IP, "ip"
}
