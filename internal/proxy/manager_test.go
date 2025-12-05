package proxy_test

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"llm-proxy/internal/proxy"
)

// Fake exec.Command that doesn't spawn real processes.
func fakeCmd() func(name string, arg ...string) *exec.Cmd {
	return func(name string, arg ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperFakeProcess")
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
}

// The fake process simply exits immediately.
func TestHelperFakeProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}

//
// ────────────────────────────────────────────────
// TESTS
// ────────────────────────────────────────────────
//

func TestLLMManager_EnsureModel_Unknown(t *testing.T) {
	m := proxy.New([]proxy.ModelConfig{}, time.Minute)

	_, err := m.EnsureModel("nope")
	if !errors.Is(err, proxy.ErrUnknownModel) {
		t.Fatalf("expected ErrUnknownModel, got: %v", err)
	}
}

func TestLLMManager_EnsureModel_StartsModel(t *testing.T) {
	restoreExec := proxy.SetExecCommand(fakeCmd())
	defer restoreExec()

	restorePortReady := proxy.SetPortReady(func(port int) bool { return false })
	defer restorePortReady()

	m := proxy.New([]proxy.ModelConfig{
		{
			Name: "test",
			Path: "/tmp/model",
			Args: []string{"--x"},
			Port: 9999,
		},
	}, time.Minute)

	port, err := m.EnsureModel("test")

	if port != 0 {
		t.Fatalf("expected port=0 while starting, got %d", port)
	}
	if !errors.Is(err, proxy.ErrModelStarting) {
		t.Fatalf("expected ErrModelStarting, got %v", err)
	}

	if m.ActiveModel() == nil || m.ActiveModel().Cfg().Name != "test" {
		t.Fatalf("model not marked active")
	}
}

func TestLLMManager_EnsureModel_ReturnsPortWhenRunning(t *testing.T) {
	restoreExec := proxy.SetExecCommand(fakeCmd())
	defer restoreExec()

	restorePortReady := proxy.SetPortReady(func(port int) bool { return false }) // model NOT ready yet
	defer restorePortReady()

	m := proxy.New([]proxy.ModelConfig{
		{Name: "test", Path: "x", Port: 7777},
	}, time.Minute)

	// First call → starts model (still "starting")
	_, _ = m.EnsureModel("test")

	// NOW mark port as ready
	restorePortReady2 := proxy.SetPortReady(func(port int) bool { return true })
	defer restorePortReady2()

	// Second call should now return the actual port
	port, err := m.EnsureModel("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 7777 {
		t.Fatalf("expected port=7777, got %d", port)
	}
}

func TestLLMManager_RecordActivity(t *testing.T) {
	restoreExec := proxy.SetExecCommand(fakeCmd())
	defer restoreExec()

	restorePortReady := proxy.SetPortReady(func(port int) bool { return false })
	defer restorePortReady()

	m := proxy.New([]proxy.ModelConfig{
		{Name: "test", Path: "x", Port: 4444},
	}, time.Millisecond*200)

	_, _ = m.EnsureModel("test")

	old := m.ActiveModel().LastUsed()
	time.Sleep(10 * time.Millisecond)

	m.RecordActivity("test")

	if !m.ActiveModel().LastUsed().After(old) {
		t.Fatalf("RecordActivity did not update timestamp")
	}
}

func TestLLMManager_IdleReaperStopsModel(t *testing.T) {
	restoreExec := proxy.SetExecCommand(fakeCmd())
	defer restoreExec()

	restorePortReady := proxy.SetPortReady(func(port int) bool { return true })
	defer restorePortReady()

	m := proxy.NewWithReapInterval(
		[]proxy.ModelConfig{
			{Name: "test", Path: "x", Port: 3333},
		},
		time.Millisecond*50, // idle timeout
		time.Millisecond*20, // reaper tick
	)
	// start model
	_, _ = m.EnsureModel("test")

	time.Sleep(time.Millisecond * 120) // wait past idle timeout

	if m.ActiveModel() != nil {
		t.Fatalf("model should be reaped due to idle timeout")
	}
}
