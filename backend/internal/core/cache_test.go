package core

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func newTestCache() *TTLCache[string, string] {
	return NewTTLCache[string, string](0, 0, nil)
}

func TestTTLCache_Get(t *testing.T) {
	c := newTestCache()

	t.Run("calls readFn on first get", func(t *testing.T) {
		var count atomic.Int32
		got, err := c.Get("k1", func() (string, error) {
			count.Add(1)
			return "v1", nil
		})
		if err != nil || got != "v1" {
			t.Fatalf("expected v1, got %q err %v", got, err)
		}
		if count.Load() != 1 {
			t.Errorf("expected 1 read, got %d", count.Load())
		}
	})

	t.Run("returns cached on second get", func(t *testing.T) {
		var count atomic.Int32
		got, err := c.Get("k1", func() (string, error) {
			count.Add(1)
			return "v2", nil
		})
		if err != nil || got != "v1" {
			t.Fatalf("expected cached v1, got %q err %v", got, err)
		}
		if count.Load() != 0 {
			t.Errorf("expected 0 reads (cached), got %d", count.Load())
		}
	})

	t.Run("different keys are independent", func(t *testing.T) {
		var count atomic.Int32
		got, err := c.Get("k2", func() (string, error) {
			count.Add(1)
			return "v2", nil
		})
		if err != nil || got != "v2" {
			t.Fatalf("expected v2, got %q err %v", got, err)
		}
		if count.Load() != 1 {
			t.Errorf("expected 1 read for new key, got %d", count.Load())
		}
	})
}

func TestTTLCache_Get_NoCache(t *testing.T) {
	// ErrNoCache delivers the value for this call but leaves the slot empty,
	// so a subsequent Get reloads instead of serving a cached hit.
	c := newTestCache()
	var loads atomic.Int32
	load := func() (string, error) {
		loads.Add(1)
		return "v", ErrNoCache
	}
	got, err := c.Get("k", load)
	if err != nil {
		t.Fatalf("expected nil err with ErrNoCache, got %v", err)
	}
	if got != "v" {
		t.Fatalf("expected delivered value v, got %q", got)
	}
	if c.Len() != 0 {
		t.Fatalf("expected no stored entry, len=%d", c.Len())
	}
	if _, err := c.Get("k", load); err != nil {
		t.Fatalf("second get: %v", err)
	}
	if loads.Load() != 2 {
		t.Fatalf("expected reload (no caching), got %d loads", loads.Load())
	}
}

func TestTTLCache_Get_ErrorPropagation(t *testing.T) {
	c := newTestCache()
	want := errors.New("read error")
	_, err := c.Get("err-key", func() (string, error) {
		return "", want
	})
	if err != want {
		t.Fatalf("expected error %v, got %v", want, err)
	}
}

func TestTTLCache_Invalidate(t *testing.T) {
	c := newTestCache()

	c.Get("k", func() (string, error) { return "v", nil })
	c.Invalidate("k")

	var count atomic.Int32
	got, err := c.Get("k", func() (string, error) {
		count.Add(1)
		return "v2", nil
	})
	if err != nil || got != "v2" {
		t.Fatalf("expected v2 after invalidation, got %q err %v", got, err)
	}
	if count.Load() != 1 {
		t.Errorf("expected 1 read after invalidation, got %d", count.Load())
	}
}

func TestTTLCache_Clear(t *testing.T) {
	c := newTestCache()

	c.Get("a", func() (string, error) { return "va", nil })
	c.Get("b", func() (string, error) { return "vb", nil })
	c.Clear()

	var count atomic.Int32
	for _, k := range []string{"a", "b"} {
		c.Get(k, func() (string, error) {
			count.Add(1)
			return "new", nil
		})
	}
	if count.Load() != 2 {
		t.Errorf("expected 2 reads after clear, got %d", count.Load())
	}
}

func TestTTLCache_TTLExpiry(t *testing.T) {
	c := NewTTLCache[string, string](0, time.Millisecond, nil)
	var loads atomic.Int32
	load := func() (string, error) { loads.Add(1); return "v", nil }

	if _, err := c.Get("k", load); err != nil {
		t.Fatalf("first get: %v", err)
	}
	if _, err := c.Get("k", load); err != nil {
		t.Fatalf("second get: %v", err)
	}
	if loads.Load() != 1 {
		t.Fatalf("expected 1 load before expiry, got %d", loads.Load())
	}

	time.Sleep(5 * time.Millisecond)
	if _, err := c.Get("k", load); err != nil {
		t.Fatalf("third get: %v", err)
	}
	if loads.Load() != 2 {
		t.Fatalf("expected reload after TTL expiry, got %d loads", loads.Load())
	}
}

func TestTTLCache_ValidFuncInvalidation(t *testing.T) {
	// Simulates PL-3's mtime check: the entry is reloaded whenever the
	// predicate reports the underlying source changed.
	var source int64
	valid := func(_ string, v string) bool { return v == "gen-"+itoa(atomic.LoadInt64(&source)) }
	c := NewTTLCache[string, string](0, time.Hour, valid)

	atomic.StoreInt64(&source, 1)
	got, err := c.Get("k", func() (string, error) { return "gen-1", nil })
	if err != nil || got != "gen-1" {
		t.Fatalf("expected gen-1, got %q err %v", got, err)
	}

	// Source unchanged -> cached.
	if g, _ := c.Get("k", func() (string, error) { t.Fatal("should be cached"); return "", nil }); g != "gen-1" {
		t.Fatalf("expected cached gen-1, got %q", g)
	}

	// Source changed -> reload.
	atomic.StoreInt64(&source, 2)
	got, err = c.Get("k", func() (string, error) { return "gen-2", nil })
	if err != nil || got != "gen-2" {
		t.Fatalf("expected reload to gen-2, got %q err %v", got, err)
	}
}

func TestTTLCache_MaxEntriesEviction(t *testing.T) {
	c := NewTTLCache[string, string](2, 0, nil)
	c.Put("a", "va")
	c.Put("b", "vb")
	c.Put("c", "vc")
	if c.Len() != 2 {
		t.Fatalf("expected 2 entries after eviction, got %d", c.Len())
	}
	if c.Contains("a") {
		t.Error("expected oldest entry 'a' evicted")
	}
	if !c.Contains("c") {
		t.Error("expected newest entry 'c' retained")
	}
}

func TestTTLCache_Reaper(t *testing.T) {
	c := NewTTLCache[string, string](0, time.Millisecond, nil)
	c.Start(time.Millisecond)
	defer c.Stop()

	c.Put("k", "v")
	time.Sleep(10 * time.Millisecond)
	if c.Len() != 0 {
		t.Fatalf("expected reaper to remove stale entry, len=%d", c.Len())
	}
}

func TestTTLCache_ConcurrentSafety(t *testing.T) {
	c := newTestCache()
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			c.Get("shared", func() (string, error) { return "ok", nil })
			c.Invalidate("shared")
			c.Clear()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
