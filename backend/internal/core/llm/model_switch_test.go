package llm_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

// slowExitCmd launches a helper that takes ~400 ms to exit after SIGTERM, like a llama-server freeing a large model, and
// records every launch so a test can ask whether the previous server was still alive when the next one started.
type launches struct {
	mu   sync.Mutex
	pids []int
	// overlapped is set when a launch happens while an earlier helper is still alive.
	overlapped bool
}

func (l *launches) cmd() func(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		l.mu.Lock()
		defer l.mu.Unlock()
		for _, pid := range l.pids {
			if syscall.Kill(pid, 0) == nil {
				l.overlapped = true
			}
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperSlowExitProcess")
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	}
}

// record notes the pid of the server the manager just started.
func (l *launches) record(m *llm.LLMRuntimeManager) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if a := m.ActiveModel(); a != nil && a.Cmd != nil && a.Cmd.Process != nil {
		l.pids = append(l.pids, a.Cmd.Process.Pid)
	}
}

func (l *launches) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.pids)
}

func TestHelperSlowExitProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)
	select {
	case <-stop:
		time.Sleep(400 * time.Millisecond)
	case <-time.After(60 * time.Second):
	}
	os.Exit(0)
}

// B1 (audit 2026-11-26): saving a model's settings stops its server; a request that arrives before the old server has
// exited must not start a second one on the same port.
func TestUpdateModel_NextStartWaitsForTheOldServerToExit(t *testing.T) {
	l := &launches{}
	defer utils.SetExecCommandContext(l.cmd())()
	defer utils.SetPortReady(func(int) bool { return false })()
	setupModelFile(t, "/tmp/model-b1.gguf")

	m := llm.New([]models.ModelConfig{{Name: "a", Path: "/tmp/model-b1.gguf", Port: 9101}}, "127.0.0.1", time.Minute)
	defer m.Shutdown()

	if _, err := m.EnsureModel(context.Background(), "a"); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("EnsureModel(a) = %v, want ErrModelStarting", err)
	}
	l.record(m)

	// A launch-relevant change restarts the server.
	if err := m.UpdateModel(models.ModelConfig{Name: "a", Path: "/tmp/model-b1.gguf", Port: 9101, Args: []string{"--threads", "4"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnsureModel(context.Background(), "a"); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("EnsureModel(a) after update = %v, want ErrModelStarting", err)
	}
	l.record(m)

	if l.count() != 2 {
		t.Fatalf("launches = %d, want 2", l.count())
	}
	if l.overlapped {
		t.Fatal("the new server was started while the old one was still running on the same port")
	}
}

// B2 (audit 2026-11-26): a settings save that changes nothing about how a model is launched (the System config handler
// pushes the same environment to every model, e.g. on a log-level change) must not restart it.
func TestUpdateModel_SameLaunchKeepsTheServerRunning(t *testing.T) {
	defer utils.SetExecCommandContext(fakeCmd())()
	defer utils.SetPortReady(func(int) bool { return false })()
	setupModelFile(t, "/tmp/model-b2.gguf")

	cfg := models.ModelConfig{Name: "a", Path: "/tmp/model-b2.gguf", Port: 9102, Environment: map[string]string{"X": "1"}}
	m := llm.New([]models.ModelConfig{cfg}, "127.0.0.1", time.Minute)
	defer m.Shutdown()

	if _, err := m.EnsureModel(context.Background(), "a"); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("EnsureModel(a) = %v, want ErrModelStarting", err)
	}
	before := m.ActiveModel()

	for _, listed := range m.ListModels() {
		if err := m.UpdateModel(listed); err != nil {
			t.Fatal(err)
		}
	}
	if m.ActiveModel() != before {
		t.Fatal("an unchanged model was restarted")
	}

	changed := cfg
	changed.Environment = map[string]string{"X": "2"}
	if err := m.UpdateModel(changed); err != nil {
		t.Fatal(err)
	}
	if m.ActiveModel() != nil {
		t.Fatal("a changed environment must restart the server")
	}
}

// B3 (audit 2026-11-26): two callers asking for different local models stopped each other's server before either was
// ready. A model that is still loading is left to finish; the other caller is told to wait.
func TestGetInstance_DoesNotStopAModelThatIsStillLoading(t *testing.T) {
	defer utils.SetExecCommandContext(fakeCmd())()
	defer utils.SetPortReady(func(int) bool { return false })()
	setupModelFile(t, "/tmp/model-b3a.gguf")
	setupModelFile(t, "/tmp/model-b3b.gguf")

	m := llm.New([]models.ModelConfig{
		{Name: "a", Path: "/tmp/model-b3a.gguf", Port: 9103},
		{Name: "b", Path: "/tmp/model-b3b.gguf", Port: 9104},
	}, "127.0.0.1", time.Minute)
	defer m.Shutdown()

	if _, err := m.EnsureModel(context.Background(), "a"); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("EnsureModel(a) = %v, want ErrModelStarting", err)
	}
	if _, err := m.EnsureModel(context.Background(), "b"); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("EnsureModel(b) = %v, want ErrModelStarting (wait)", err)
	}
	if a := m.ActiveModel(); a == nil || a.Cfg.Name != "a" {
		t.Fatal("the loading model a must be left to finish")
	}
}
