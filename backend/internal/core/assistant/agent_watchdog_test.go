package assistant

import (
	"context"
	"testing"
	"time"

	"llm-proxy/internal/platform/logging"
)

func TestWatchdog_Fires(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// GlobalTimeout 0 + tiny grace => watchdog fires almost immediately.
	a := &Agent{
		config: AgentConfig{GlobalTimeout: 0},
		deps:   AgentRuntimeDeps{Logger: logging.NewNopLogger()},
	}
	a.startWatchdogGrace(ctx, cancel, 20*time.Millisecond)

	select {
	case <-ctx.Done():
		// expected: watchdog cancelled the context
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not force-cancel the context")
	}
}

func TestWatchdog_NoFalsePositive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // context already done before watchdog starts
	a := &Agent{
		config: AgentConfig{GlobalTimeout: 0},
		deps:   AgentRuntimeDeps{Logger: logging.NewNopLogger()},
	}

	done := make(chan struct{})
	go func() {
		a.startWatchdogGrace(ctx, cancel, 20*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		// expected: goroutine exits via execCtx.Done() without re-firing
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog goroutine leaked instead of exiting on already-done context")
	}
}
