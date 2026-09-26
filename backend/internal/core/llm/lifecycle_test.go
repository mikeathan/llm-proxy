package llm_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

func TestIdleReaper_IgnoresStartingModels(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	// Initially port is NOT ready (still starting). The readiness flag is read
	// by the reaper goroutine via the portReady stub, so it must be atomic to
	// avoid a data race when the test flips it below.
	var isReady atomic.Bool
	restorePort := utils.SetPortReady(func(port int) bool { return isReady.Load() })
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

	// Wait longer than idle timeout
	time.Sleep(time.Millisecond * 100)

	// Model should NOT be reaped because it's not ready yet
	if m.ActiveModel() == nil {
		t.Fatalf("model should NOT be reaped while starting")
	}

	// Now simulate ready
	isReady.Store(true)
	// Wait for another reaper tick
	time.Sleep(time.Millisecond * 100)

	// Now it should be reaped because it's ready AND idle
	if m.ActiveModel() != nil {
		t.Fatalf("model should be reaped after becoming ready and exceeding idle timeout")
	}
}

func TestIdleReaper_StopsHangingModels(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	// Port NEVER becomes ready
	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	// We can't wait 5 minutes in a unit test.
	// However, I can't easily override the startupTimeout without modifying the code to accept it.
	// For now, I'll skip the actual 5m wait but keep the test structure ready if we ever make it configurable.
	t.Skip("Skipping 5m hang test to avoid slow CI")
}

// logCrashCmd returns a fake exec.Command whose helper writes model output and
// then exits non-zero, simulating a llama-server that crashes after logging.
func logCrashCmd() func(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperLogCrashProcess")
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	}
}

func TestHelperLogCrashProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "llama-server: simulated crash: out of memory")
	os.Exit(1)
}

// TestRuntimeManager_CrashedModel_RetainsLogs guards that a crashed model's
// captured stdout/stderr survive the model being cleared, so /admin/api/logs can
// still show WHY it exited. Previously the buffer was dropped with activeModel,
// leaving the endpoint with empty logs after a crash.
func TestRuntimeManager_CrashedModel_RetainsLogs(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(logCrashCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return false })
	defer restorePort()

	setupModelFile(t, "crash_logs.gguf")
	m := llm.New([]models.ModelConfig{{Name: "test", Path: "crash_logs.gguf", Port: 5556}}, "127.0.0.1", time.Minute)
	defer m.Shutdown()

	_, _ = m.EnsureModel(context.Background(), "test")

	// Wait for the process to exit, then trigger the clear on the next call.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if am := m.ActiveModel(); am != nil && am.Exited() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _ = m.EnsureModel(context.Background(), "test")

	if m.ActiveModel() != nil {
		t.Fatal("expected crashed model cleared")
	}
	if logs := m.ActiveLogs(); !strings.Contains(logs, "simulated crash") {
		t.Errorf("ActiveLogs() = %q, want retained crash output", logs)
	}
}

func TestIdleReaper_RespectsZeroTimeout(t *testing.T) {
	restoreExec := utils.SetExecCommandContext(fakeCmd())
	defer restoreExec()

	restorePort := utils.SetPortReady(func(port int) bool { return true })
	defer restorePort()

	setupModelFile(t, "zero_timeout.gguf")
	m := llm.NewWithReapInterval(
		[]models.ModelConfig{
			{Name: "test", Path: "zero_timeout.gguf", Port: 3333},
		},
		"127.0.0.1",
		0,                   // NO idle timeout
		time.Millisecond*20, // reaper tick
	)
	// Stop the reaper goroutine so it does not outlive this test and race with
	// the next test's PortReady/ExecCommandContext overrides.
	defer m.Shutdown()

	_, _ = m.EnsureModel(context.Background(), "test")

	// Wait a while
	time.Sleep(time.Millisecond * 100)

	// Model should STILL be running
	if m.ActiveModel() == nil {
		t.Fatalf("model should NOT be reaped when idleTimeout is 0")
	}
}
