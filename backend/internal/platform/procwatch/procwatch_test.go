package procwatch

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWatch_DetectsExit(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	w := New(cmd)

	waitDone(t, w)
	if !w.Exited() {
		t.Fatal("expected Exited() true after the process terminated")
	}
	if w.Err() == nil {
		t.Fatal("expected Err() to report a non-nil exit error")
	}
	if !strings.Contains(w.Err().Error(), "exit status 1") {
		t.Errorf("expected 'exit status 1' in Err(), got: %v", w.Err())
	}
}

func TestWatch_NotExitedWhileAlive(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	w := New(cmd)

	if w.Exited() {
		t.Fatal("expected Exited() false while the process is alive")
	}
	if w.Err() != nil {
		t.Errorf("expected Err() nil while alive, got %v", w.Err())
	}

	// Clean up via the watch's own done channel (single Wait owner).
	_ = cmd.Process.Kill()
	waitDone(t, w)
}

func waitDone(t *testing.T, w *Watch) {
	t.Helper()
	select {
	case <-w.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("expected watch to observe process exit")
	}
}
