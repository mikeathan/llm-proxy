package ratelimiter

import (
	"llm-proxy/utils"
	"sync"
	"time"
)

type Limiter interface {
	Allow(key string, interval time.Duration) bool
	Clear()
}

type rateLimiter struct {
	mu    *sync.Mutex
	calls map[string]time.Time
	clock utils.Clock
}

func NewLimiter(clock utils.Clock) Limiter {
	return &rateLimiter{mu: &sync.Mutex{}, calls: make(map[string]time.Time), clock: clock}
}

func (l *rateLimiter) Allow(key string, interval time.Duration) bool {

	now := l.clock.NowUtc()
	l.mu.Lock()
	defer l.mu.Unlock()

	last, exists := l.calls[key]
	if exists && now.Sub(last) < interval {
		return false
	}

	l.calls[key] = now
	return true
}

func (l *rateLimiter) Clear() {
	l.mu.Lock()
	l.calls = make(map[string]time.Time)
	l.mu.Unlock()
}
