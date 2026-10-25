package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy/internal/platform/sandbox"
	"llm-proxy/models"
)

func TestPreflight_OnlyTheAuditorIsChecked(t *testing.T) {
	deps := readyDeps()
	deps.Config = func() models.NetauditConfig { return models.NetauditConfig{} } // scanning off
	r := NewReadiness(deps)

	if _, _, refused := r.Preflight(context.Background(), "task-runner"); refused {
		t.Error("an ordinary automation must never be refused for a missing scan scope")
	}
	code, message, refused := r.Preflight(context.Background(), AuditorProfileID)
	if !refused || code != string(models.RequirementScopeEmpty) || !strings.Contains(message, "No network has been allowed") {
		t.Errorf("auditor with no scope: %q %q %v", code, message, refused)
	}
}

func TestPreflight_AnAuditCannotRunWithoutNmapEvenThoughAChatCan(t *testing.T) {
	deps := readyDeps()
	deps.Nmap = fakeNmap{err: errors.New("nmap is not installed")}
	r := NewReadiness(deps)
	if len(r.Blocking()) != 0 {
		t.Fatal("a missing nmap does not hide the tool from an attended chat")
	}
	code, _, refused := r.Preflight(context.Background(), AuditorProfileID)
	if !refused || code != string(models.RequirementNmapMissing) {
		t.Errorf("the nightly audit must stop on a missing nmap: %q %v", code, refused)
	}
}

func TestPreflight_ReportsEveryProblemInOneMessage(t *testing.T) {
	deps := readyDeps()
	deps.Config = func() models.NetauditConfig { return models.NetauditConfig{} }
	deps.Nmap = fakeNmap{err: errors.New("gone")}
	deps.Sandbox = fakeSB{m: sandbox.EnforcementNone}
	_, message, refused := NewReadiness(deps).Preflight(context.Background(), AuditorProfileID)
	if !refused || !strings.Contains(message, "No network has been allowed") || !strings.Contains(message, "nmap is not available") {
		t.Errorf("message = %q", message)
	}
	if _, _, refused := NewReadiness(readyDeps()).Preflight(context.Background(), AuditorProfileID); refused {
		t.Error("a ready host passes")
	}
}
