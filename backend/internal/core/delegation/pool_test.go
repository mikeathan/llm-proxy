package delegation

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The pool admits up to its limit, read live; a full pool makes the next task wait for a release, give up after its
// wait, or stop with its context.
func TestPool(t *testing.T) {
	limit := 2
	p := newPool(func() int { return limit })
	ctx := context.Background()
	r1, err1 := p.acquire(ctx, time.Second)
	r2, err2 := p.acquire(ctx, time.Second)
	if err1 != nil || err2 != nil {
		t.Fatalf("two slots: %v, %v", err1, err2)
	}
	if _, err := p.acquire(ctx, 20*time.Millisecond); !errors.Is(err, errPoolBusy) {
		t.Errorf("a full pool: err = %v, want busy", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.acquire(cancelled, time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled task: err = %v, want canceled", err)
	}

	got := make(chan error, 1)
	go func() {
		release, err := p.acquire(ctx, time.Second)
		if err == nil {
			release()
		}
		got <- err
	}()
	time.Sleep(10 * time.Millisecond)
	r1()
	if err := <-got; err != nil {
		t.Errorf("a waiting task gets the freed slot: %v", err)
	}

	limit = 3 // raised live
	r3, err := p.acquire(ctx, 20*time.Millisecond)
	if err != nil {
		t.Errorf("a raised limit admits at once: %v", err)
	}
	r2()
	r3()
	if p.inUse != 0 {
		t.Errorf("in use after every release = %d", p.inUse)
	}
}
