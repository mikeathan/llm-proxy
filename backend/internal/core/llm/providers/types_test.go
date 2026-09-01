package providers

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

// TestRunningModel_StartWatch_DetectsCrash verifies the exit-watch goroutine
// (the persistentShell reuse pattern) detects a process that terminates — the
// mechanism that turns a llama-server crash on launch into a detected failure.
func TestRunningModel_StartWatch_DetectsCrash(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	rm := &RunningModel{Cfg: models.ModelConfig{Name: "crash"}}
	rm.Cmd = cmd
	rm.StartWatch()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if rm.Exited() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !rm.Exited() {
		t.Fatal("expected Exited() to become true after the process terminated")
	}
	if rm.Err() == nil {
		t.Error("expected Err() to report a non-nil exit error")
	}
	if !strings.Contains(rm.Err().Error(), "exit status 1") {
		t.Errorf("expected 'exit status 1' in Err(), got: %v", rm.Err())
	}
}

// TestRunningModel_NotExitedBeforeProcessDies verifies Exited() is false while
// the process is still running, so a healthy starting model is not misflagged.
func TestRunningModel_NotExitedBeforeProcessDies(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	rm := &RunningModel{Cmd: cmd}
	rm.StartWatch()

	if rm.Exited() {
		t.Fatal("expected Exited() to be false while the process is alive")
	}
	if rm.Err() != nil {
		t.Errorf("expected Err() nil while alive, got %v", rm.Err())
	}

	// Clean up: kill and wait via the watchdog's own channel (the single owner
	// of cmd.Wait — never call cmd.Wait a second time).
	_ = cmd.Process.Kill()
	select {
	case <-rm.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("expected watchdog to observe the killed process")
	}
}
