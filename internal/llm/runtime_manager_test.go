package llm_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/llm"
	"llm-proxy/internal/testhooks"
	"llm-proxy/models"
)

// Fake exec.Command that doesn't spawn a real process.
func fakeCmd() func(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperFakeProcess")
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
}

func TestHelperFakeProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}

// --- Config and lifecycle behavior tests ---

func TestNewManagerFromConfig_NormalizesModels(t *testing.T) {
	cfg := &models.Config{
		Server: models.ServerConfig{
			ModelHost:   "127.0.0.1",
			DefaultArgs: []string{"--alpha", "1"},
		},
		ModelDir: "/models",
		Models: []models.ModelConfig{
			{Name: "m1", Filename: "model.gguf", Args: []string{"--beta", "2"}, Port: 9000},
		},
	}

	manager := llm.NewManagerFromConfig(cfg)
	modelsOut := manager.ListModels()
	if len(modelsOut) != 1 {
		t.Fatalf("expected 1 model, got %d", len(modelsOut))
	}
	m := modelsOut[0]
	if m.Path != filepath.Join(cfg.ModelDir, "model.gguf") {
		t.Fatalf("unexpected path: %s", m.Path)
	}
	expectedArgs := []string{"--alpha", "1", "--beta", "2"}
	if !equalStrings(m.Args, expectedArgs) {
		t.Fatalf("unexpected args: %#v", m.Args)
	}
}

func TestEnsureModel_AssignsPortAndReturnsReadyInstance(t *testing.T) {
	restoreExec := testhooks.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := testhooks.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf"}}, "127.0.0.1", time.Minute)

	_, err := manager.EnsureModel("test")
	if !errors.Is(err, llm.ErrModelStarting) {
		t.Fatalf("expected ErrModelStarting, got %v", err)
	}

	modelsOut := manager.ListModels()
	if len(modelsOut) != 1 || modelsOut[0].Port == 0 {
		t.Fatalf("expected model port to be assigned")
	}
	assignedPort := modelsOut[0].Port

	restorePort()
	restorePort = testhooks.SetPortReady(func(port int) bool { return true })

	inst, err := manager.EnsureModel("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Port != assignedPort {
		t.Fatalf("expected port %d, got %d", assignedPort, inst.Port)
	}
}

func TestUpdateModel_StopsActiveModel(t *testing.T) {
	restoreExec := testhooks.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := testhooks.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf", Port: 9001}}, "127.0.0.1", time.Minute)
	_, _ = manager.EnsureModel("test")

	if manager.ActiveModel() == nil {
		t.Fatalf("expected active model")
	}

	if err := manager.UpdateModel(models.ModelConfig{Name: "test", Path: "/tmp/model.gguf", Port: 9002}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manager.ActiveModel() != nil {
		t.Fatalf("expected active model to be stopped")
	}
}

func TestRemoveModel_StopsActiveModel(t *testing.T) {
	restoreExec := testhooks.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := testhooks.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf", Port: 9001}}, "127.0.0.1", time.Minute)
	_, _ = manager.EnsureModel("test")

	if manager.ActiveModel() == nil {
		t.Fatalf("expected active model")
	}

	if err := manager.RemoveModel("test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manager.ActiveModel() != nil {
		t.Fatalf("expected active model to be stopped")
	}
}

func TestStopActive_CancelsProcessContext(t *testing.T) {
	var captured context.Context
	restoreExec := testhooks.SetExecCommandContext(func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		captured = ctx
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperFakeProcess")
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	})
	defer restoreExec()

	restorePort := testhooks.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf"}}, "127.0.0.1", time.Minute)
	_, _ = manager.EnsureModel("test")

	if captured == nil {
		t.Fatal("expected context to be captured")
	}

	if err := manager.StopActive(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-captured.Done():
	default:
		t.Fatal("expected command context to be canceled")
	}
}

func TestActiveInfo_ReadyReflectsPortState(t *testing.T) {
	restoreExec := testhooks.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := testhooks.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf", Port: 9001}}, "127.0.0.1", time.Minute)
	_, _ = manager.EnsureModel("test")

	info := manager.ActiveInfo()
	if info == nil || info.Ready {
		t.Fatalf("expected Ready=false when port not ready")
	}

	restorePort()
	restorePort = testhooks.SetPortReady(func(port int) bool { return true })

	info = manager.ActiveInfo()
	if info == nil || !info.Ready {
		t.Fatalf("expected Ready=true when port ready")
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// --- Existing runtime manager tests ---

func TestRuntimeManager_EnsureModel_Unknown(t *testing.T) {
	m := llm.New(nil, "127.0.0.1", time.Minute)

	_, err := m.EnsureModel("nope")
	if !errors.Is(err, llm.ErrUnknownModel) {
		t.Fatalf("expected ErrUnknownModel, got: %v", err)
	}
}

func TestRuntimeManager_EnsureModel_StartsModel(t *testing.T) {
	restoreExec := testhooks.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := testhooks.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	m := llm.New([]models.ModelConfig{
		{Name: "test", Path: "/tmp/model.gguf", Args: []string{"--x"}, Port: 9999},
	}, "127.0.0.1", time.Minute)

	mi, err := m.EnsureModel("test")

	if mi.Port != 0 {
		t.Fatalf("expected empty ModelInstance (Port=0) while starting, got: %+v", mi)
	}
	if !errors.Is(err, llm.ErrModelStarting) {
		t.Fatalf("expected ErrModelStarting, got: %v", err)
	}

	if m.ActiveModel() == nil || m.ActiveModel().Cfg().Name != "test" {
		t.Fatalf("model should be active")
	}
}

func TestRuntimeManager_EnsureModel_ReturnsInstanceWhenReady(t *testing.T) {
	restoreExec := testhooks.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := testhooks.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	m := llm.New([]models.ModelConfig{
		{Name: "test", Path: "model.gguf", Port: 7777},
	}, "127.0.0.1", time.Minute)

	// First call: starting
	_, _ = m.EnsureModel("test")

	// Now simulate ready
	restorePort2 := testhooks.SetPortReady(func(port int) bool { return true })
	defer restorePort2()

	mi, err := m.EnsureModel("test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mi.Port != 7777 {
		t.Fatalf("expected port=7777, got %d", mi.Port)
	}
	if mi.Host != "127.0.0.1" {
		t.Fatalf("expected host=127.0.0.1, got %s", mi.Host)
	}
	if mi.Name != "test" {
		t.Fatalf("expected Name=test, got %s", mi.Name)
	}
}

func TestRuntimeManager_RecordActivity(t *testing.T) {
	restoreExec := testhooks.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := testhooks.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	m := llm.New([]models.ModelConfig{
		{Name: "test", Path: "x", Port: 4444},
	}, "127.0.0.1", time.Millisecond*200)

	_, _ = m.EnsureModel("test")

	old := m.ActiveModel().LastUsed()
	time.Sleep(10 * time.Millisecond)

	m.RecordActivity("test")

	if !m.ActiveModel().LastUsed().After(old) {
		t.Fatalf("RecordActivity should update lastUsed")
	}
}

func TestRuntimeManager_IdleReaperStopsModel(t *testing.T) {
	restoreExec := testhooks.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := testhooks.SetPortReady(func(port int) bool { return true })
	defer restorePort()

	m := llm.NewWithReapInterval(
		[]models.ModelConfig{
			{Name: "test", Path: "x", Port: 3333},
		},
		"127.0.0.1",
		time.Millisecond*50, // idle timeout
		time.Millisecond*20, // reaper tick
	)

	_, _ = m.EnsureModel("test")

	time.Sleep(time.Millisecond * 120)

	if m.ActiveModel() != nil {
		t.Fatalf("model should be reaped after idle timeout")
	}
}
