// Package process provides cross-platform process listing and killing
// for model server binaries (e.g. llama-server). Used by the admin UI
// to detect and clean up orphaned processes that survived a proxy crash.
package process

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Info struct {
	PID     int       `json:"pid"`
	Binary  string    `json:"binary"`
	Model   string    `json:"model"`
	Port    int       `json:"port"`
	Started time.Time `json:"started"`
	Uptime  string    `json:"uptime"`
	Active  bool      `json:"active"`
}

// ListByBinary returns all running processes whose binary name contains the
// given fragment (e.g. "llama-server"). The activePID is compared against
// each found PID to set the Active flag.
func ListByBinary(binaryName string, activePID int) ([]Info, error) {
	return listByBinary(binaryName, activePID)
}

// isAlive checks if a process with the given PID is still running on Unix.
func isAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true // Process is running
	}
	if errors.Is(err, syscall.EPERM) {
		return true // Process running, but we lack permission
	}
	return false
}

// Kill sends SIGTERM to the process, waits up to 5 seconds for it to exit,
// and sends SIGKILL if it is still alive.
func Kill(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}

	// Send SIGTERM
	if err := p.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil // Already dead
		}
		return fmt.Errorf("signal TERM %d: %w", pid, err)
	}

	// Poll up to 5 seconds (50 * 100ms) for the process to exit
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if !isAlive(pid) {
			return nil
		}
	}

	// Still alive, send SIGKILL
	if err := p.Signal(syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil // Died just before we killed it
		}
		return fmt.Errorf("signal KILL %d: %w", pid, err)
	}

	// Poll briefly to ensure SIGKILL took effect
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if !isAlive(pid) {
			return nil
		}
	}

	return fmt.Errorf("failed to kill process %d: still alive after SIGKILL", pid)
}

// splitExecutable finds the executable at the start of a ps command line whose
// base name contains binaryName, and returns it with the remaining arguments.
// ps does not quote, so an absolute path containing spaces spans several
// fields: later fields are joined on only while they continue a path (they do
// not start with "/" or "-") and only when the joined base name is exactly
// binaryName, so an interpreter running a script is never taken for the server.
func splitExecutable(command, binaryName string) (string, []string, bool) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", nil, false
	}
	if strings.Contains(filepath.Base(fields[0]), binaryName) {
		return fields[0], fields[1:], true
	}
	if !strings.HasPrefix(fields[0], "/") {
		return "", nil, false
	}
	for k := 1; k < len(fields) && !strings.HasPrefix(fields[k], "/") && !strings.HasPrefix(fields[k], "-"); k++ {
		joined := strings.Join(fields[:k+1], " ")
		if filepath.Base(joined) == binaryName {
			return joined, fields[k+1:], true
		}
	}
	return "", nil, false
}

func parseArgs(args []string) (model string, port int) {
	for i, a := range args {
		if a == "-m" && i+1 < len(args) {
			model = args[i+1]
			if idx := strings.LastIndex(model, "/"); idx >= 0 {
				model = model[idx+1:]
			}
			if idx := strings.LastIndex(model, "\\"); idx >= 0 {
				model = model[idx+1:]
			}
		}
		if a == "--port" && i+1 < len(args) {
			port, _ = strconv.Atoi(args[i+1])
		}
	}
	return
}

func uptime(started time.Time) string {
	d := time.Since(started).Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func readCmdline(path string) (string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	parts := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, fmt.Errorf("empty cmdline")
	}
	return parts[0], parts, nil
}
