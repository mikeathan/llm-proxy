package safe

import (
	"testing"
	"time"
)

func TestGo_ContainsPanic(t *testing.T) {
	done := make(chan struct{})
	Go("test panic", func() {
		panic("boom")
	})

	// The panic must be contained: a bare `go` would crash the test process.
	Go("test ok", func() { close(done) })
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Go did not run the healthy goroutine")
	}
}

func TestGo_DoesNotSwallowReturnValues(t *testing.T) {
	// Verify fn's effects (outside the panic path) are preserved.
	out := make(chan string, 1)
	Go("test value", func() { out <- "ok" })
	select {
	case v := <-out:
		if v != "ok" {
			t.Fatalf("unexpected value %q", v)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Go did not deliver the goroutine's result")
	}
}
