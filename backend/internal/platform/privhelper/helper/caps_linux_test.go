//go:build linux

package helper

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// statusMask returns one capability mask ("CapPrm", "CapEff") of the process whose /proc/<pid>/status path is given.
func statusMask(t *testing.T, path, key string) uint64 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, key+":"); ok {
			n, err := strconv.ParseUint(strings.TrimSpace(v), 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
	}
	t.Fatalf("no %s in %s", key, path)
	return 0
}

// childCapEff starts a program from the calling thread and returns the effective capability mask it reports for itself.
func childCapEff(t *testing.T) uint64 {
	t.Helper()
	out, err := exec.Command("sh", "-c", "grep ^CapEff: /proc/self/status").Output()
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(string(out), "CapEff:")), 16, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// The proof the design asks for (D3): the program that asks systemd who the main process is holds no capability, and a verified action's
// program holds CAP_NET_RAW and nothing else. It runs under a unit with AmbientCapabilities=CAP_NET_RAW (scripts/check-helper-under-unit.sh) and
// is skipped elsewhere, because nothing can be proved about a capability the process never had.
func TestThreadAmbientTakesTheCapabilityOffAndPutsItBackForChildren(t *testing.T) {
	const netRaw = uint64(1) << CapNetRaw
	if statusMask(t, "/proc/self/status", "CapPrm")&netRaw == 0 {
		t.Skip("this process was not given CAP_NET_RAW; run it under a unit with AmbientCapabilities=CAP_NET_RAW")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Capabilities belong to a thread, and a child inherits those of the thread that forks it.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var a ThreadAmbient
		if err := a.Clear(); err != nil {
			t.Error(err)
			return
		}
		if got := childCapEff(t); got != 0 {
			t.Errorf("a child started after Clear must hold no capability, CapEff=%x", got)
		}
		if err := a.Raise(CapNetRaw); err != nil {
			t.Error(err)
			return
		}
		if got := childCapEff(t); got != netRaw {
			t.Errorf("a child started after Raise must hold CAP_NET_RAW and nothing else, CapEff=%x", got)
		}
		if err := a.Clear(); err != nil {
			t.Error(err)
			return
		}
		if got := childCapEff(t); got != 0 {
			t.Errorf("Clear must take it away again, CapEff=%x", got)
		}
	}()
	<-done
}

func TestRaisingACapabilityTheProcessWasNeverGivenFails(t *testing.T) {
	if statusMask(t, "/proc/self/status", "CapPrm") != 0 {
		t.Skip("this process holds capabilities")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := (ThreadAmbient{}).Raise(CapNetRaw); err == nil {
			t.Error("raising a capability that is not permitted must fail, not silently do nothing")
		}
	}()
	<-done
}
