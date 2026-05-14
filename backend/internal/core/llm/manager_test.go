package llm_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/testing/utils"
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

func setupModelFile(t *testing.T, path string) {
	t.Helper()
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create model dir: %v", err)
		}
	}
	if err := os.WriteFile(path, []byte("fake model"), 0644); err != nil {
		t.Fatalf("failed to create fake model file: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })
}

// --- Config and lifecycle behavior tests ---

func TestNewManagerFromConfig_NormalizesModels(t *testing.T) {
	cfg := &models.Config{
		Server: models.ServerConfig{
			ModelHost:   "127.0.0.1",
			DefaultArgs: []string{"--alpha", "1"},
		},
		Providers: map[string]models.ProviderItem{
			"local": {ModelDir: "/models"},
		},
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
	localDir := cfg.Providers["local"].ModelDir
	if m.Path != filepath.Join(localDir, "model.gguf") {
		t.Fatalf("unexpected path: %s", m.Path)
	}
	expectedArgs := []string{"--beta", "2"}
	if !equalStrings(m.Args, expectedArgs) {
		t.Fatalf("unexpected args: %#v", m.Args)
	}
}

func TestEnsureModel_AssignsPortAndReturnsReadyInstance(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "/tmp/model.gguf")
	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf"}}, "127.0.0.1", time.Minute)

	_, err := manager.EnsureModel(context.Background(), "test")
	if !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("expected ErrModelStarting, got %v", err)
	}

	modelsOut := manager.ListModels()
	if len(modelsOut) != 1 || modelsOut[0].Port == 0 {
		t.Fatalf("expected model port to be assigned")
	}
	assignedPort := modelsOut[0].Port

	restorePort()
	restorePort = utils.SetPortReady(func(port int) bool { return true })

	inst, err := manager.EnsureModel(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Port != assignedPort {
		t.Fatalf("expected port %d, got %d", assignedPort, inst.Port)
	}
}

func TestUpdateModel_StopsActiveModel(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "/tmp/model.gguf")
	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf", Port: 9001}}, "127.0.0.1", time.Minute)
	_, _ = manager.EnsureModel(context.Background(), "test")

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
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "/tmp/model.gguf")
	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf", Port: 9001}}, "127.0.0.1", time.Minute)
	_, _ = manager.EnsureModel(context.Background(), "test")

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
	restoreExec := utils.SetExecCommandContext(func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		captured = ctx
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperFakeProcess")
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	})
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "/tmp/model.gguf")
	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf"}}, "127.0.0.1", time.Minute)
	_, _ = manager.EnsureModel(context.Background(), "test")

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
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "/tmp/model.gguf")
	manager := llm.New([]models.ModelConfig{{Name: "test", Path: "/tmp/model.gguf", Port: 9001}}, "127.0.0.1", time.Minute)
	_, _ = manager.EnsureModel(context.Background(), "test")

	info := manager.ActiveInfo()
	if info == nil || info.Ready {
		t.Fatalf("expected Ready=false when port not ready")
	}

	restorePort()
	restorePort = utils.SetPortReady(func(port int) bool { return true })

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

	_, err := m.EnsureModel(context.Background(), "nope")
	if !errors.Is(err, llm.ErrUnknownModel) {
		t.Fatalf("expected ErrUnknownModel, got: %v", err)
	}
}

func TestRuntimeManager_EnsureModel_StartsModel(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "/tmp/model.gguf")
	m := llm.New([]models.ModelConfig{
		{Name: "test", Path: "/tmp/model.gguf", Args: []string{"--x"}, Port: 9999},
	}, "127.0.0.1", time.Minute)

	mi, err := m.EnsureModel(context.Background(), "test")

	if mi.Port != 0 {
		t.Fatalf("expected empty ModelInstance (Port=0) while starting, got: %+v", mi)
	}
	if !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("expected ErrModelStarting, got: %v", err)
	}

	if m.ActiveModel() == nil || m.ActiveModel().Cfg.Name != "test" {
		t.Fatalf("model should be active")
	}
}

func TestRuntimeManager_EnsureModel_ReturnsInstanceWhenReady(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "model.gguf")
	m := llm.New([]models.ModelConfig{
		{Name: "test", Path: "model.gguf", Port: 7777},
	}, "127.0.0.1", time.Minute)

	// First call: starting
	_, _ = m.EnsureModel(context.Background(), "test")

	// Now simulate ready
	restorePort2 := utils.SetPortReady(func(port int) bool { return true })
	defer restorePort2()

	mi, err := m.EnsureModel(context.Background(), "test")

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
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "activity.gguf")
	m := llm.New([]models.ModelConfig{
		{Name: "test", Path: "activity.gguf", Port: 4444},
	}, "127.0.0.1", time.Millisecond*200)

	_, _ = m.EnsureModel(context.Background(), "test")

	old := m.ActiveModel().LastUsed
	time.Sleep(10 * time.Millisecond)

	m.RecordActivity("test")

	if !m.ActiveModel().LastUsed.After(old) {
		t.Fatalf("RecordActivity should update lastUsed")
	}
}

func TestRuntimeManager_IdleReaperStopsModel(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return true })
	defer restorePort()

	setupModelFile(t, "reap_test.gguf")
	m := llm.NewWithReapInterval(
		[]models.ModelConfig{
			{Name: "test", Path: "reap_test.gguf", Port: 3333},
		},
		"127.0.0.1",
		time.Millisecond*50, // idle timeout
		time.Millisecond*20, // reaper tick
	)

	_, _ = m.EnsureModel(context.Background(), "test")

	time.Sleep(time.Millisecond * 120)

	if m.ActiveModel() != nil {
		t.Fatalf("model should be reaped after idle timeout")
	}
}
