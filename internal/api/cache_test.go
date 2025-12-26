package api

import (
	"llm-proxy/internal/mocks"
	"testing"
	"time"
)

func TestDeviceContextCache_Empty(t *testing.T) {
	clock := mocks.NewFakeClock(time.Now())
	cache := NewDeviceContextCache(10*time.Minute, clock)

	ctx, ok := cache.Get()
	if ok {
		t.Fatalf("expected cache miss, got hit: %+v", ctx)
	}
}

func TestDeviceContextCache_SetAndGet(t *testing.T) {
	clock := mocks.NewFakeClock(time.Now())
	cache := NewDeviceContextCache(10*time.Minute, clock)

	expected := &DeviceContextResponse{
		Version: "1",
	}

	cache.Set(expected)

	ctx, ok := cache.Get()
	if !ok {
		t.Fatalf("expected cache hit")
	}

	if ctx != expected {
		t.Fatalf("expected same pointer back from cache")
	}
}

func TestDeviceContextCache_Expires(t *testing.T) {
	start := time.Now()
	clock := mocks.NewFakeClock(start)
	cache := NewDeviceContextCache(5*time.Minute, clock)

	cache.Set(&DeviceContextResponse{Version: "1"})

	clock.Advance(6 * time.Minute)

	ctx, ok := cache.Get()
	if ok {
		t.Fatalf("expected cache miss after TTL, got %+v", ctx)
	}
}

func TestDeviceContextCache_Invalidate(t *testing.T) {
	clock := mocks.NewFakeClock(time.Now())
	cache := NewDeviceContextCache(10*time.Minute, clock)

	cache.Set(&DeviceContextResponse{Version: "1"})
	cache.Invalidate()

	ctx, ok := cache.Get()
	if ok {
		t.Fatalf("expected cache miss after invalidate, got %+v", ctx)
	}
}
