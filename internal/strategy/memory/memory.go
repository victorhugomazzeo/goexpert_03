package memory

import (
	"context"
	"sync"
	"time"

	"github.com/victorhugomazzeo/goexpert_03/internal/limiter"
)

var _ limiter.Strategy = (*Store)(nil)

type entry struct {
	hits         int64
	windowEnds   time.Time
	blockedUntil time.Time
}

type Store struct {
	mu       sync.Mutex
	data     map[string]*entry
	now      func() time.Time
	stop     chan struct{}
	stopOnce sync.Once
}

func New() *Store {
	return NewWithClock(time.Now)
}

func NewWithClock(now func() time.Time) *Store {
	s := &Store{
		data: make(map[string]*entry),
		now:  now,
		stop: make(chan struct{}),
	}

	go s.janitor(time.Minute)

	return s
}

func (s *Store) Check(ctx context.Context, key string, limit limiter.Limit, window time.Duration) (limiter.State, error) {

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	e, exists := s.data[key]
	if !exists {
		e = &entry{}
		s.data[key] = e
	}

	if e.blockedUntil.After(now) {
		return limiter.State{
			Allowed:    false,
			Blocked:    true,
			Hits:       e.hits,
			RetryAfter: e.blockedUntil.Sub(now),
		}, nil
	}

	if !e.blockedUntil.IsZero() {
		e.blockedUntil = time.Time{}
		e.hits = 0
		e.windowEnds = time.Time{}
	}

	if !now.Before(e.windowEnds) {
		e.hits = 0
		e.windowEnds = now.Add(window)
	}

	e.hits++

	if e.hits > int64(limit.MaxRequests) {
		e.blockedUntil = now.Add(limit.BlockDuration)
		return limiter.State{
			Allowed:    false,
			Blocked:    false,
			Hits:       e.hits,
			RetryAfter: limit.BlockDuration,
		}, nil
	}

	return limiter.State{
		Allowed:    true,
		Blocked:    false,
		Hits:       e.hits,
		RetryAfter: e.windowEnds.Sub(now),
	}, nil
}

func (s *Store) Reset(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, key)
	return nil
}

func (s *Store) Close() error {
	s.stopOnce.Do(func() {
		close(s.stop)
	})

	return nil
}

func (s *Store) sweep() {

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	for key, e := range s.data {
		if e.blockedUntil.Before(now) && e.windowEnds.Before(now) {
			delete(s.data, key)
		}
	}
}

func (s *Store) janitor(interval time.Duration) {

	ticker := time.NewTicker(interval)

	for {
		select {
		case <-ticker.C:
			s.sweep()
		case <-s.stop:
			ticker.Stop()
			return
		}
	}
}
