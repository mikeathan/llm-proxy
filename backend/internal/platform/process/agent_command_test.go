package process

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

// agentCommandCallers are the only files that may start an agent child with AgentCommand: the shell, the one-shot
// terminal command and the guarded-program runner. Every one of them wraps the command in the sandbox provider.
// Adding a spawn site is a security decision, so it is a change to this list.
var agentCommandCallers = []string{
	"internal/core/tools/terminal.go",
	"internal/platform/progexec/progexec.go",
	"internal/shell/terminal.go",
}

func TestAgentCommandHasOnlyTheKnownCallers(t *testing.T) {
	root := backendRoot(t)
	fset := token.NewFileSet()
	found := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == "frontend_dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "AgentCommand" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "process" {
				rel, _ := filepath.Rel(root, path)
				found[filepath.ToSlash(rel)] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for file := range found {
		if !slices.Contains(agentCommandCallers, file) {
			t.Errorf("%s starts an agent child with process.AgentCommand but is not a known spawn site", file)
		}
	}
	for _, file := range agentCommandCallers {
		if !found[file] {
			t.Errorf("%s is listed as a spawn site but no longer calls process.AgentCommand; remove it from the list", file)
		}
	}
}

func backendRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("backend root (go.mod) not found")
		}
		dir = parent
	}
}
