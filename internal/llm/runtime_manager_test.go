package llm_test

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"llm-proxy/internal/llm"
	"llm-proxy/internal/testhooks"
	"llm-proxy/models"
)

// Fake exec.Command that doesn't spawn a real process.
func fakeCmd() func(name string, arg ...string) *exec.Cmd {
	return func(name string, arg ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperFakeProcess")
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
}

// Helper process stub
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
	m := llm.New(nil, "127.0.0.1", time.Minute)

	_, err := m.EnsureModel("nope")
	if !errors.Is(err, llm.ErrUnknownModel) {
		t.Fatalf("expected ErrUnknownModel, got: %v", err)
	}
}

func TestLLMManager_EnsureModel_StartsModel(t *testing.T) {
	restoreExec := testhooks.SetExecCommand(fakeCmd())
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

func TestLLMManager_EnsureModel_ReturnsInstanceWhenReady(t *testing.T) {
	restoreExec := testhooks.SetExecCommand(fakeCmd())
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

func TestLLMManager_RecordActivity(t *testing.T) {
	restoreExec := testhooks.SetExecCommand(fakeCmd())
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

func TestLLMManager_IdleReaperStopsModel(t *testing.T) {
	restoreExec := testhooks.SetExecCommand(fakeCmd())
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
