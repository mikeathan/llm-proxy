package assistant

import (
	"context"
	"errors"
	"testing"

	"llm-proxy/internal/core/proxy"
)

// mockStopGuard is a configurable StopGuard for unit-testing maybeNudge.
type mockStopGuard struct {
	nudge string
	err   error
}

func (g *mockStopGuard) Nudge(s *runSession) (string, error) {
	return g.nudge, g.err
}

// newTestSession builds a runSession with a no-op agent for maybeNudge tests.
func newTestSession() *runSession {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5})
	return newRunSession(agent, context.Background(), []proxy.Message{{Role: proxy.UserRole, Content: "task"}})
}

func TestMaybeNudge_NoGuards(t *testing.T) {
	s := newTestSession()
	if nudge, ok := s.maybeNudge(); ok {
		t.Fatalf("expected no nudge with no guards, got %q", nudge)
	}
	if s.stopGuardAttempts != 0 {
		t.Errorf("expected stopGuardAttempts untouched, got %d", s.stopGuardAttempts)
	}
	if s.finalizeAttempts != 0 {
		t.Errorf("finalizeAttempts must stay untouched by maybeNudge, got %d", s.finalizeAttempts)
	}
}

func TestMaybeNudge_FirstNudge(t *testing.T) {
	s := newTestSession()
	s.stopGuards = []StopGuard{&mockStopGuard{nudge: "review your work"}}
	nudge, ok := s.maybeNudge()
	if !ok {
		t.Fatal("expected a nudge")
	}
	if nudge != "review your work" {
		t.Errorf("unexpected nudge content %q", nudge)
	}
	if s.stopGuardAttempts != 1 {
		t.Errorf("expected stopGuardAttempts 1, got %d", s.stopGuardAttempts)
	}
}

func TestMaybeNudge_BoundedByCap(t *testing.T) {
	s := newTestSession()
	s.stopGuards = []StopGuard{&mockStopGuard{nudge: "nudge"}}
	for i := 0; i < maxStopGuardAttempts; i++ {
		if _, ok := s.maybeNudge(); !ok {
			t.Fatalf("expected nudge at attempt %d", i+1)
		}
	}
	if nudge, ok := s.maybeNudge(); ok {
		t.Fatalf("expected nudge suppressed past the cap, got %q", nudge)
	}
	if s.stopGuardAttempts != maxStopGuardAttempts {
		t.Errorf("expected stopGuardAttempts capped at %d, got %d", maxStopGuardAttempts, s.stopGuardAttempts)
	}
}

func TestMaybeNudge_GuardAllows(t *testing.T) {
	s := newTestSession()
	s.stopGuards = []StopGuard{&mockStopGuard{nudge: ""}}
	if nudge, ok := s.maybeNudge(); ok {
		t.Fatalf("expected no nudge when the guard allows, got %q", nudge)
	}
	if s.stopGuardAttempts != 0 {
		t.Errorf("stopGuardAttempts must not increment when the guard allows, got %d", s.stopGuardAttempts)
	}
}

func TestMaybeNudge_GuardErrorAllows(t *testing.T) {
	s := newTestSession()
	s.stopGuards = []StopGuard{&mockStopGuard{err: errors.New("boom")}}
	if nudge, ok := s.maybeNudge(); ok {
		t.Fatalf("expected finalization allowed on guard error, got %q", nudge)
	}
	if s.stopGuardAttempts != 0 {
		t.Errorf("stopGuardAttempts must not increment on guard error, got %d", s.stopGuardAttempts)
	}
}
