package core

import (
	"errors"
	"sync/atomic"
	"testing"
)

func TestContentCache_Get(t *testing.T) {
	var c ContentCache

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

func TestContentCache_Get_ErrorPropagation(t *testing.T) {
	var c ContentCache
	want := errors.New("read error")
	_, err := c.Get("err-key", func() (string, error) {
		return "", want
	})
	if err != want {
		t.Fatalf("expected error %v, got %v", want, err)
	}
}

func TestContentCache_Invalidate(t *testing.T) {
	var c ContentCache

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

func TestContentCache_Clear(t *testing.T) {
	var c ContentCache

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

func TestContentCache_ConcurrentSafety(t *testing.T) {
	var c ContentCache
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
