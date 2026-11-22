package newsroom

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// blockingModel answers when released and counts the calls that reached it and the most that ran at once.
type blockingModel struct {
	release        chan struct{}
	err            error
	calls, running atomic.Int32
	mostAtOnce     atomic.Int32
	entered        chan struct{}
}

func newBlockingModel() *blockingModel {
	return &blockingModel{release: make(chan struct{}), entered: make(chan struct{}, 8)}
}

func (m *blockingModel) Complete(ctx context.Context, _ stacksdk.ModelRequest) (stacksdk.ModelResponse, error) {
	m.calls.Add(1)
	now := m.running.Add(1)
	defer m.running.Add(-1)
	for {
		seen := m.mostAtOnce.Load()
		if now <= seen || m.mostAtOnce.CompareAndSwap(seen, now) {
			break
		}
	}
	m.entered <- struct{}{}
	select {
	case <-m.release:
		return stacksdk.ModelResponse{Text: "ok"}, m.err
	case <-ctx.Done():
		return stacksdk.ModelResponse{}, ctx.Err()
	}
}

func TestModelGate_SecondCallerWaitsInsteadOfBeingRefused(t *testing.T) {
	inner := newBlockingModel()
	g := newModelGate(inner)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := g.Complete(context.Background(), stacksdk.ModelRequest{User: "q"})
			results <- err
		}()
	}
	<-inner.entered // the first call is in the model; the second must be waiting, not inside it
	time.Sleep(20 * time.Millisecond)
	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("the second caller must wait for the first, but %d calls reached the model", got)
	}
	inner.release <- struct{}{}
	<-inner.entered
	inner.release <- struct{}{}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("a caller that waited its turn must be answered, got %v", err)
		}
	}
	if inner.mostAtOnce.Load() != 1 {
		t.Errorf("at most one call may be in the model at once, saw %d", inner.mostAtOnce.Load())
	}
}

func TestModelGate_CancelledWaitReturnsWithoutCallingTheModel(t *testing.T) {
	inner := newBlockingModel()
	g := newModelGate(inner)
	go func() { _, _ = g.Complete(context.Background(), stacksdk.ModelRequest{User: "first"}) }()
	<-inner.entered
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := g.Complete(ctx, stacksdk.ModelRequest{User: "second"})
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if stacksdk.CodeOf(err) != stacksdk.CodeTimeout {
			t.Fatalf("a cancelled wait is a timeout the caller can handle, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("a cancelled wait must return promptly")
	}
	if got := inner.calls.Load(); got != 1 {
		t.Errorf("the cancelled caller must never reach the model, %d calls did", got)
	}
	close(inner.release)
}

func TestModelGate_RefusalsPassThroughUnchanged(t *testing.T) {
	inner := newBlockingModel()
	denied := stacksdk.Errorf(stacksdk.CodeDenied, "the limit is spent")
	inner.err = denied
	close(inner.release)
	g := newModelGate(inner)
	_, err := g.Complete(context.Background(), stacksdk.ModelRequest{User: "q"})
	if stacksdk.CodeOf(err) != stacksdk.CodeDenied || err.Error() != denied.Error() {
		t.Fatalf("a refusal must reach the caller as it was, got %v", err)
	}
}

func TestModelGate_SlotIsFreedAfterAFailedCall(t *testing.T) {
	inner := newBlockingModel()
	inner.err = errors.New("boom")
	close(inner.release)
	g := newModelGate(inner)
	for range 3 {
		go func() { <-inner.entered }()
		if _, err := g.Complete(context.Background(), stacksdk.ModelRequest{User: "q"}); err == nil {
			t.Fatal("the model's failure must reach the caller")
		}
	}
}
