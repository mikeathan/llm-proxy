package llm_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

// Fake exec.Command that doesn't spawn a real llama-server.
func fakeCmd() func(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperFakeProcess")
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		// Isolate into its own process group so that signalStopLocked's
		// group-kill (-pgid) only ever signals the helper, never the test binary.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	}
}

func TestHelperFakeProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// Simulate a long-running model server: stay alive until signalled (or a
	// bounded fallback if a test forgets to reap it). A real llama-server must
	// not exit while active, and the exit-watch goroutine treats any exit as a
	// crash — so a helper that exits immediately would look like a failed model.
	time.Sleep(60 * time.Second)
}

// crashCmd returns a fake exec.Command whose helper exits immediately with a
// non-zero status, simulating a llama-server that fails to launch (bad args).
func crashCmd() func(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperCrashProcess")
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	}
}

func TestHelperCrashProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(1)
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
	defer manager.Shutdown()

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
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
	defer manager.Shutdown()
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
	defer m.Shutdown()

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
	defer m.Shutdown()

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
	defer m.Shutdown()

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
	// Stop the reaper goroutine so it does not outlive this test and race with
	// the next test's PortReady/ExecCommandContext overrides.
	defer m.Shutdown()

	_, _ = m.EnsureModel(context.Background(), "test")

	time.Sleep(time.Millisecond * 120)

	if m.ActiveModel() != nil {
		t.Fatalf("model should be reaped after idle timeout")
	}
}

// TestRuntimeManager_CrashedModel_SurfacesError verifies that a local model
// whose process dies (bad args, missing model) is detected: GetInstance returns
// a clear failure (not ErrModelStarting forever) and the dead model is cleared
// so the next request starts a fresh one.
func TestRuntimeManager_CrashedModel_SurfacesError(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(crashCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "crash.gguf")
	m := llm.New([]models.ModelConfig{{Name: "test", Path: "crash.gguf", Port: 5555}}, "127.0.0.1", time.Minute)
	defer m.Shutdown()

	_, err := m.EnsureModel(context.Background(), "test")
	if !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("first call expected ErrModelStarting, got %v", err)
	}

	// Wait for the watchdog to observe the process exiting.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if am := m.ActiveModel(); am != nil && am.Exited() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	_, err = m.EnsureModel(context.Background(), "test")
	if err == nil || errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("expected a start-failure error, got %v", err)
	}
	if !strings.Contains(err.Error(), "failed to start") {
		t.Errorf("expected 'failed to start' in error, got: %v", err)
	}
	if m.LastModelError() == "" {
		t.Error("expected LastModelError to be set after a crash")
	}
	if m.ActiveModel() != nil {
		t.Errorf("expected crashed model to be cleared, got activeModel %+v", m.ActiveModel())
	}
}

// TestRuntimeManager_CrashedModel_ReapedAndRecorded verifies the idle reaper
// detects a crashed process and clears it promptly (not after the 5-minute
// startup timeout), recording the failure for the status/UI.
func TestRuntimeManager_CrashedModel_ReapedAndRecorded(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(crashCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "crash_reap.gguf")
	m := llm.NewWithReapInterval(
		[]models.ModelConfig{{Name: "test", Path: "crash_reap.gguf", Port: 3333}},
		"127.0.0.1",
		time.Hour,             // idle timeout: not the trigger here
		time.Millisecond*20,   // reaper tick
	)
	defer m.Shutdown()

	_, _ = m.EnsureModel(context.Background(), "test")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m.ActiveModel() == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if m.ActiveModel() != nil {
		t.Fatal("expected crashed model to be reaped promptly")
	}
	if m.LastModelError() == "" {
		t.Error("expected LastModelError to be set after crash reaping")
	}
}

// TestSync_ReconcilesLocalServingContextFromLaunchArgs verifies that Sync
// reconciles a manager-launched local model's serving-context metadata with
// the -c/--ctx-size it will actually be launched with, so a stale GGUF
// n_ctx_train snapshot cannot inflate the derived budgets (observed: model
// saved while llama-server was down kept n_ctx 262144 while the launch used
// --ctx-size 16384, yielding max_tokens 87381 and a ~28-minute reasoning
// budget).

// TestReconcileLocalServingContext verifies the runtime refreshes a local
// model's serving-context metadata from its RUNNING server and re-derives
// budgets from the corrected window — so a stale GGUF n_ctx_train snapshot
// (262144 stored while the launch serves 16384) cannot inflate max_tokens /
// reasoning budgets.
func TestReconcileLocalServingContext(t *testing.T) {
	var slotsHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slots":
			atomic.AddInt32(&slotsHits, 1)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":0,"n_ctx":16384}]`))
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"object":"list","data":[{"id":"Qwen_Qwen3.5-35B-A3B-Q4_K_M.gguf","object":"model","owned_by":"llamacpp","meta":{"n_ctx":16384,"n_ctx_train":262144}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("parse httptest port: %v", err)
	}

	m := llm.NewWithReapInterval(nil, "127.0.0.1", time.Minute, time.Hour)
	if err := m.AddModel(models.ModelConfig{
		Name:     "qwen",
		Provider: models.ProviderLocal,
		Filename: "Qwen_Qwen3.5-35B-A3B-Q4_K_M.gguf",
		Path:     "/models/Qwen_Qwen3.5-35B-A3B-Q4_K_M.gguf",
		Port:     port,
		Metadata: &models.ModelMetadata{Nctx: 262144, ContextLength: 262144},
	}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}

	m.ReconcileLocalServingContext(context.Background(), "qwen")

	cfg := findModel(t, m, "qwen")
	if cfg.Metadata == nil || cfg.Metadata.Nctx != 16384 {
		t.Fatalf("metadata n_ctx = %+v, want 16384 (running server window)", cfg.Metadata)
	}
	if want := 16384 / 3; cfg.MaxTokens != want {
		t.Errorf("max_tokens = %d, want %d (derived from serving ctx 16384)", cfg.MaxTokens, want)
	}

	// A second call must not re-probe (once per server start) and must not
	// change the reconciled metadata.
	before := atomic.LoadInt32(&slotsHits)
	m.ReconcileLocalServingContext(context.Background(), "qwen")
	if got := atomic.LoadInt32(&slotsHits); got != before {
		t.Errorf("reconcile ran again after success: /slots hits %d -> %d", before, got)
	}
	if cfg2 := findModel(t, m, "qwen"); cfg2.Metadata.Nctx != 16384 || cfg2.MaxTokens != 16384/3 {
		t.Errorf("metadata drifted on second reconcile: n_ctx=%d max_tokens=%d", cfg2.Metadata.Nctx, cfg2.MaxTokens)
	}
}
