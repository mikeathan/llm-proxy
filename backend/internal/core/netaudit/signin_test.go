package netaudit

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSignInAlerts_EachKindIsToldAtMostOncePerWindow(t *testing.T) {
	clock := &monitorClock{at: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	var mu sync.Mutex
	var sent []string
	a := NewSignInAlerts(func(_ context.Context, m string) error {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, m)
		return nil
	}, clock.now)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()

	a.Tell(SignInLockedOut)
	a.Tell(SignInLockedOut) // within the window: dropped
	a.Tell(SignInFromNewAddress)
	clock.add(signInAlertEvery + time.Second)
	a.Tell(SignInLockedOut)
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		n := len(sent)
		mu.Unlock()
		if n >= 3 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if len(sent) != 3 {
		t.Fatalf("sent = %q, want three (lockout, new address, lockout after the window)", sent)
	}
	if !strings.Contains(sent[0], "locked") || !strings.Contains(sent[1], "never signed in") {
		t.Errorf("messages = %q", sent)
	}
	for _, m := range sent {
		if strings.Contains(m, "10.") {
			t.Errorf("no address may be in the message: %q", m)
		}
	}
}

func TestSignInAlerts_TellNeverBlocksWithoutTheLoop(t *testing.T) {
	a := NewSignInAlerts(func(context.Context, string) error { return nil }, time.Now)
	done := make(chan struct{})
	go func() {
		for range 100 {
			a.Tell(SignInLockedOut)
			a.Tell(SignInFromNewAddress)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Tell blocked on the sign-in path")
	}
}
