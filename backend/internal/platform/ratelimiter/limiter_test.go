package ratelimiter_test

import (
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/internal/platform/ratelimiter"
	"testing"
	"time"
)

func TestLimiter_FirstCallAllowed(t *testing.T) {
	clock := mocks.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := ratelimiter.NewLimiter(clock)

	if !limiter.Allow("key", time.Second) {
		t.Fatalf("expected first call to be allowed")
	}
}

func TestLimiter_SecondCallBlockedWithinInterval(t *testing.T) {
	clock := mocks.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := ratelimiter.NewLimiter(clock)

	if !limiter.Allow("key", time.Second) {
		t.Fatalf("expected first call to be allowed")
	}
	if limiter.Allow("key", time.Second) {
		t.Fatalf("expected second call to be blocked")
	}
}

func TestLimiter_AllowedAfterInterval(t *testing.T) {
	clock := mocks.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := ratelimiter.NewLimiter(clock)

	limiter.Allow("key", time.Second)

	clock.Advance(1 * time.Second)

	if !limiter.Allow("key", time.Second) {
		t.Fatalf("expected call to be allowed after interval")
	}
}

func TestLimiter_PerKeyIsolation(t *testing.T) {
	clock := mocks.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := ratelimiter.NewLimiter(clock)

	if !limiter.Allow("keyA", time.Second) {
		t.Fatalf("expected keyA to be allowed")
	}
	if !limiter.Allow("keyB", time.Second) {
		t.Fatalf("expected keyB to be allowed independently")
	}
}

func TestLimiter_ClearResetsState(t *testing.T) {
	clock := mocks.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := ratelimiter.NewLimiter(clock)

	limiter.Allow("key", time.Hour)
	limiter.Clear()

	if !limiter.Allow("key", time.Hour) {
		t.Fatalf("expected call to be allowed after clear")
	}
}
