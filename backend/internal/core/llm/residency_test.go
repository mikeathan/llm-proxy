package llm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

// fakeGuard stands in for the run scheduler's model gate.
type fakeGuard struct {
	label     string
	active    string
	requested string
}

func (g *fakeGuard) BlockedBy(active, requested, callerKey string) string {
	g.active, g.requested = active, requested
	return g.label
}

func TestEnsureModel_RefusesToEvictAModelInUse(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "/tmp/model-a.gguf")
	setupModelFile(t, "/tmp/model-b.gguf")

	manager := llm.New([]models.ModelConfig{
		{Name: "a", Path: "/tmp/model-a.gguf", Port: 9001},
		{Name: "b", Path: "/tmp/model-b.gguf", Port: 9002},
	}, "127.0.0.1", time.Minute)
	defer manager.Shutdown()

	// Bring a up so it is the active local model.
	if _, err := manager.EnsureModel(context.Background(), "a"); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("EnsureModel(a) = %v, want ErrModelStarting", err)
	}
	if manager.ActiveModel() == nil {
		t.Fatal("expected model a to be active")
	}

	guard := &fakeGuard{label: "ws/automation"}
	manager.SetResidencyGuard(guard)

	_, err := manager.EnsureModel(context.Background(), "b")
	if !errors.Is(err, llm.ErrLocalModelBusy) {
		t.Fatalf("EnsureModel(b) = %v, want ErrLocalModelBusy", err)
	}
	active := manager.ActiveModel()
	if active == nil || active.Cfg.Name != "a" {
		t.Fatal("the running model must be left alone when the switch is refused")
	}
	if guard.active != "a" || guard.requested != "b" {
		t.Fatalf("guard saw active=%q requested=%q, want a -> b", guard.active, guard.requested)
	}

	// Once the blocker releases, the switch proceeds.
	guard.label = ""
	if _, err := manager.EnsureModel(context.Background(), "b"); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("EnsureModel(b) after release = %v, want ErrModelStarting", err)
	}
}

func TestEnsureModel_NoGuardLeavesEvictionUnchanged(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "/tmp/model-a.gguf")
	setupModelFile(t, "/tmp/model-b.gguf")

	manager := llm.New([]models.ModelConfig{
		{Name: "a", Path: "/tmp/model-a.gguf", Port: 9003},
		{Name: "b", Path: "/tmp/model-b.gguf", Port: 9004},
	}, "127.0.0.1", time.Minute)
	defer manager.Shutdown()

	if _, err := manager.EnsureModel(context.Background(), "a"); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("EnsureModel(a) = %v, want ErrModelStarting", err)
	}

	// No guard installed: switching models still works as before.
	if _, err := manager.EnsureModel(context.Background(), "b"); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("EnsureModel(b) without a guard = %v, want ErrModelStarting", err)
	}
}
