package process

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestAgentCommandSetsProcessGroup(t *testing.T) {
	cmd := AgentCommand(context.Background(), "sh", "-c", "true")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("agent children must run in their own process group (Setpgid) for kill-group semantics")
	}
}

func TestKillGroup(t *testing.T) {
	if err := KillGroup(nil, syscall.SIGTERM); err != nil {
		t.Errorf("KillGroup on nil cmd must be a no-op, got %v", err)
	}

	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	time.Sleep(100 * time.Millisecond)
	if err := KillGroup(cmd, syscall.SIGKILL); err != nil {
		t.Fatalf("KillGroup: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process group not killed within timeout")
	}
	// The grandchild sleep must be gone too: look for it by pgid. The orphan is
	// reaped asynchronously by init after the group kill, and a not-yet-reaped
	// zombie still answers kill(pgid, 0), so poll briefly for the group to
	// actually drain instead of racing the reaper (observable under CI load).
	deadline := time.Now().Add(5 * time.Second)
	for pidAlive(-cmd.Process.Pid) {
		if time.Now().After(deadline) {
			t.Fatal("grandchild survived KillGroup")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// pidAlive reports whether a (possibly negative = group) pid still exists.
func pidAlive(pid int) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil
}
