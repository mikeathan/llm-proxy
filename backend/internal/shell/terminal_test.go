package shell

import (
	"context"
	"io"
	"llm-proxy/internal/platform/sandbox"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestHostShellManager_Execution(t *testing.T) {
	ctx := context.Background()
	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("failed to create HostShellManager: %v", err)
	}
	defer hm.Shutdown(context.Background())

	tmpDir, _ := os.MkdirTemp("", "shell-test-*")
	defer os.RemoveAll(tmpDir)

	_ = os.WriteFile(filepath.Join(tmpDir, "hello.txt"), []byte("hello shell"), 0644)

	t.Run("Create and Execute", func(t *testing.T) {
		ts, err := hm.GetOrCreate(ctx, "test-ws", tmpDir, WorkspacePolicy{})
		if err != nil {
			t.Fatalf("GetOrCreate failed: %v", err)
		}

		stdout, _, err := ts.Execute(ctx, []string{"sh", "-c", "cat hello.txt"})
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		defer stdout.Close()

		data, _ := io.ReadAll(stdout)
		if strings.TrimSpace(string(data)) != "hello shell" {
			t.Errorf("expected 'hello shell', got '%s'", string(data))
		}
	})

	t.Run("Persistence Check", func(t *testing.T) {
		ts, _ := hm.GetOrCreate(ctx, "test-ws", tmpDir, WorkspacePolicy{})

		// Set a variable in one command
		_, _, _ = ts.Execute(ctx, []string{"export MYVAR=antigravity"})

		// Read it in another command
		stdout, _, _ := ts.Execute(ctx, []string{"echo $MYVAR"})
		defer stdout.Close()

		data, _ := io.ReadAll(stdout)
		if strings.TrimSpace(string(data)) != "antigravity" {
			t.Errorf("expected 'antigravity', got '%s' (persistence failed)", strings.TrimSpace(string(data)))
		}
	})

	t.Run("Reaper Test", func(t *testing.T) {
		_, _ = hm.GetOrCreate(ctx, "reap-ws", tmpDir, WorkspacePolicy{IdleTimeout: 10 * time.Millisecond})
		time.Sleep(50 * time.Millisecond)
		hm.reap() // Trigger manual reap for fast test

		// Pool keys are (workspace, networkOn) — the session must be gone
		// regardless of scope.
		for _, s := range hm.ListSessions() {
			if s.WorkspaceID == "reap-ws" {
				t.Errorf("expected session 'reap-ws' to be reaped due to inactivity, still listed: %+v", s)
			}
		}
	})

	t.Run("Environment Inheritance", func(t *testing.T) {
		// Set a custom host variable
		os.Setenv("TEST_VAR_ANTIGRAVITY", "stabilized")
		defer os.Unsetenv("TEST_VAR_ANTIGRAVITY")

		ts, _ := hm.GetOrCreate(ctx, "env-ws", tmpDir, WorkspacePolicy{AllowedEnvVars: []string{"TEST_VAR_ANTIGRAVITY", "HOME", "GOPATH", "TMPDIR"}})

		// 1. Verify inheritance via allowlist
		stdout, _, err := ts.Execute(ctx, []string{"sh", "-c", "echo $TEST_VAR_ANTIGRAVITY"})
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		defer stdout.Close()
		data, _ := io.ReadAll(stdout)
		if strings.TrimSpace(string(data)) != "stabilized" {
			t.Errorf("expected 'stabilized', got '%s'", strings.TrimSpace(string(data)))
		}

		// 2. Verify Isolation Overrides (HOME should point to jail, not real home)
		stdout, _, _ = ts.Execute(ctx, []string{"sh", "-c", "echo $HOME"})
		defer stdout.Close()
		homeData, _ := io.ReadAll(stdout)
		expectedHome := filepath.Join(tmpDir, ".sandbox")
		if strings.TrimSpace(string(homeData)) != expectedHome {
			t.Errorf("expected HOME to be jail path %s, got %s", expectedHome, strings.TrimSpace(string(homeData)))
		}

		// 3. Verify GOPATH and TMPDIR redirection
		stdout, _, _ = ts.Execute(ctx, []string{"sh", "-c", "echo $TMPDIR"})
		defer stdout.Close()
		tmpData, _ := io.ReadAll(stdout)
		expectedTmp := filepath.Join(tmpDir, ".sandbox/tmp")
		if strings.TrimSpace(string(tmpData)) != expectedTmp {
			t.Errorf("expected TMPDIR to be %s, got %s", expectedTmp, strings.TrimSpace(string(tmpData)))
		}
	})
}

func TestMergeEnv(t *testing.T) {
	host := []string{"USER=mike", "PATH=/bin", "LANG=en_US.UTF-8"}
	overrides := []string{"HOME=/jail", "PATH=/jail/bin"}

	merged := mergeEnv(host, overrides)
	envMap := make(map[string]string)
	for _, e := range merged {
		kv := strings.SplitN(e, "=", 2)
		envMap[kv[0]] = kv[1]
	}

	if envMap["USER"] != "mike" {
		t.Errorf("expected USER=mike, got %s", envMap["USER"])
	}
	if envMap["PATH"] != "/jail/bin" {
		t.Errorf("expected PATH=/jail/bin (override), got %s", envMap["PATH"])
	}
	if envMap["HOME"] != "/jail" {
		t.Errorf("expected HOME=/jail, got %s", envMap["HOME"])
	}
}

func TestPrepareShellEnv(t *testing.T) {
	os.Setenv("SENSITIVE_VAR", "secret")
	os.Setenv("ALLOWED_VAR", "public")
	defer os.Unsetenv("SENSITIVE_VAR")
	defer os.Unsetenv("ALLOWED_VAR")

	hostPath := "/tmp/jail"
	allowed := []string{"ALLOWED_VAR", "HOME"}

	env := prepareShellEnv(hostPath, allowed, nil)
	envMap := make(map[string]string)
	for _, e := range env {
		kv := strings.SplitN(e, "=", 2)
		envMap[kv[0]] = kv[1]
	}

	if _, ok := envMap["SENSITIVE_VAR"]; ok {
		t.Errorf("SENSITIVE_VAR should have been filtered out")
	}
	if envMap["ALLOWED_VAR"] != "public" {
		t.Errorf("expected ALLOWED_VAR=public, got %s", envMap["ALLOWED_VAR"])
	}
	expectedHome := hostPath + "/.sandbox"
	if envMap["HOME"] != expectedHome {
		t.Errorf("expected HOME to be overridden to %s, got %s", expectedHome, envMap["HOME"])
	}
	expectedGoCache := hostPath + "/.sandbox/go-cache"
	if envMap["GOMODCACHE"] != expectedGoCache {
		t.Errorf("expected GOMODCACHE override to %s, got %s", expectedGoCache, envMap["GOMODCACHE"])
	}
}

func TestPersistentShell_ContextCancelKillsProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ps, err := newPersistentShell(ctx, ".", nil, WorkspacePolicy{})
	if err != nil {
		t.Fatalf("newPersistentShell failed: %v", err)
	}
	defer ps.stdin.Close()

	// Start a long-running command (sleep 60s) in the shell
	execCtx, execCancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var execResult string
	var execErr error

	go func() {
		execResult, execErr = ps.Execute(execCtx, "sleep 60")
		_ = execResult
		close(done)
	}()

	// Give the command a moment to start
	time.Sleep(200 * time.Millisecond)

	// Cancel the context — Execute should return promptly (< 5s)
	execCancel()

	select {
	case <-done:
		if execErr == nil {
			t.Error("expected error after context cancellation, got nil")
		}
		if !strings.Contains(execErr.Error(), "context canceled") && !strings.Contains(execErr.Error(), "killed") {
			t.Errorf("expected context cancellation error, got: %v", execErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return within 5s of context cancellation — process was not killed")
	}
}

func TestPersistentShell_ProcessGroupIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ps, err := newPersistentShell(ctx, ".", nil, WorkspacePolicy{})
	if err != nil {
		t.Fatalf("newPersistentShell failed: %v", err)
	}
	defer ps.stdin.Close()

	if ps.cmd == nil || ps.cmd.Process == nil {
		t.Fatal("process not started")
	}

	// The bash process should be in its own process group.
	// pgid == pid when Setpgid creates a new group.
	pgid, err := syscall.Getpgid(ps.cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Getpgid failed: %v", err)
	}
	if pgid != ps.cmd.Process.Pid {
		t.Errorf("expected process group ID %d to equal PID %d (separate group)", pgid, ps.cmd.Process.Pid)
	}

	// Verify PGID method returns the negated PID (for syscall.Kill)
	pgidFromPGID := ps.PGID()
	if pgidFromPGID != -ps.cmd.Process.Pid {
		t.Errorf("PGID() returned %d, expected %d (-pid for process group)", pgidFromPGID, -ps.cmd.Process.Pid)
	}
}

func TestHostShellManager_ExecuteContextCancel(t *testing.T) {
	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("NewHostShellManager failed: %v", err)
	}
	defer hm.Shutdown(context.Background())

	tmpDir := t.TempDir()

	ts, err := hm.GetOrCreate(context.Background(), "cancel-test-ws", tmpDir, WorkspacePolicy{})
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	execCtx, execCancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		stdout, stderr, _ := ts.Execute(execCtx, []string{"sh", "-c", "sleep 60"})
		if stdout != nil {
			stdout.Close()
		}
		if stderr != nil {
			stderr.Close()
		}
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	execCancel()

	select {
	case <-done:
		// Returned promptly — success
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return within 5s of context cancellation")
	}
}

func TestHostShellManager_Shutdown_RespectsContext(t *testing.T) {
	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("NewHostShellManager failed: %v", err)
	}

	tmpDir := t.TempDir()
	if _, err := hm.GetOrCreate(context.Background(), "ctx-shutdown-ws", tmpDir, WorkspacePolicy{}); err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	// A cancelled context must make Shutdown return promptly. Shutdown derives a
	// 5s cleanup budget per session from the context; with a cancelled parent
	// the first cleanup is bounded and the loop breaks early.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		hm.Shutdown(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success — Shutdown returned without blocking on the context deadline.
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown blocked past the cancelled context deadline")
	}
}

func TestPrepareShellEnv_PathAugmentation(t *testing.T) {
	hostPath := "/tmp/jail"
	allowed := []string{"PATH"}
	extensions := []string{"node_modules/.bin", "bin"}

	// Set a mock host PATH
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "/usr/bin:/bin")
	defer os.Setenv("PATH", oldPath)

	env := prepareShellEnv(hostPath, allowed, extensions)
	envMap := make(map[string]string)
	for _, e := range env {
		kv := strings.SplitN(e, "=", 2)
		envMap[kv[0]] = kv[1]
	}

	// Host path should be preserved after extensions
	if !strings.Contains(envMap["PATH"], "/usr/bin") {
		t.Errorf("expected PATH to still contain host paths, got %s", envMap["PATH"])
	}
}

// TestPersistentShell_TrailingCommentDoesNotStall is a regression test for the
// completion-sentinel fix: a command ending in a shell comment must still emit
// the sentinel (the sentinel is newline-separated, so the comment cannot
// swallow it) and return promptly with the correct output — previously the
// sentinel line was commented out and the reader stalled until the context
// timeout, wedging the shared session.
func TestPersistentShell_TrailingCommentDoesNotStall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpDir, _ := os.MkdirTemp("", "shell-cmt-*")
	defer os.RemoveAll(tmpDir)

	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("failed to create HostShellManager: %v", err)
	}
	defer hm.Shutdown(context.Background())

	ts, err := hm.GetOrCreate(ctx, "cmt-ws", tmpDir, WorkspacePolicy{})
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	stdout, _, err := ts.Execute(ctx, []string{"echo hello # trailing comment"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	defer stdout.Close()

	data, _ := io.ReadAll(stdout)
	if strings.TrimSpace(string(data)) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

// D8: sessions pool per (workspaceID, networkOn); scope transitions never
// recycle (they key apart); Recycle drops every scope; an epoch bump recycles a
// same-key shell so it never enforces a stale policy.
func TestHostShellManager_PoolKeyedByWorkspaceNetworkOn(t *testing.T) {
	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("NewHostShellManager: %v", err)
	}
	defer hm.Shutdown(context.Background())
	tmpDir, _ := os.MkdirTemp("", "shell-d8-*")
	defer os.RemoveAll(tmpDir)

	netOff := WorkspacePolicy{NetworkOn: false}
	netOn := WorkspacePolicy{NetworkOn: true}

	sameKey := func() bool {
		a, err := hm.GetOrCreate(context.Background(), "wsA", tmpDir, netOff)
		if err != nil {
			t.Fatal(err)
		}
		b, err := hm.GetOrCreate(context.Background(), "wsA", tmpDir, netOff)
		if err != nil {
			t.Fatal(err)
		}
		return a == b
	}()
	if !sameKey {
		t.Error("same (workspace, networkOn) must return the same pooled session")
	}

	off, err := hm.GetOrCreate(context.Background(), "wsA", tmpDir, netOff)
	if err != nil {
		t.Fatal(err)
	}
	on, err := hm.GetOrCreate(context.Background(), "wsA", tmpDir, netOn)
	if err != nil {
		t.Fatal(err)
	}
	if off == on {
		t.Error("network-on and network-off must be distinct pool keys (D8)")
	}

	sessions := hm.ListSessions()
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions for wsA (net on/off), got %d", len(sessions))
	}
	seen := map[bool]bool{}
	for _, s := range sessions {
		if s.WorkspaceID != "wsA" {
			t.Errorf("unexpected workspace in view: %q", s.WorkspaceID)
		}
		seen[s.NetworkOn] = true
	}
	if !seen[false] || !seen[true] {
		t.Errorf("views must carry network_on for both scopes: %+v", sessions)
	}

	// Scope transition needs NO recycle (different keys already).
	hm.Recycle(context.Background(), "wsA")
	if got := hm.ListSessions(); len(got) != 0 {
		t.Errorf("Recycle must drop every scope for the workspace, got %+v", got)
	}
}

func TestHostShellManager_EpochBumpRecyclesSameKeyShell(t *testing.T) {
	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("NewHostShellManager: %v", err)
	}
	defer hm.Shutdown(context.Background())
	tmpDir, _ := os.MkdirTemp("", "shell-epoch-*")
	defer os.RemoveAll(tmpDir)

	first, err := hm.GetOrCreate(context.Background(), "wsE", tmpDir, WorkspacePolicy{Epoch: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Same key, newer epoch → recycled, so a fresh session is returned.
	second, err := hm.GetOrCreate(context.Background(), "wsE", tmpDir, WorkspacePolicy{Epoch: 2})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("epoch bump must recycle the same-key shell (plan risk R6)")
	}
	if got := hm.ListSessions(); len(got) != 1 {
		t.Errorf("recycle must not accumulate sessions, got %d", len(got))
	}
}

// Phase 1c: host egress-proxy env vars reach network-on shells (upper + lower
// case forms so curl/python honor them).
func TestHostShellManager_EgressProxyEnv(t *testing.T) {
	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("NewHostShellManager: %v", err)
	}
	defer hm.Shutdown(context.Background())
	tmpDir, _ := os.MkdirTemp("", "shell-egress-*")
	defer os.RemoveAll(tmpDir)

	ts, err := hm.GetOrCreate(context.Background(), "egress-ws", tmpDir, WorkspacePolicy{
		NetworkOn: true,
		ProxyEnv: []string{
			"HTTP_PROXY=http://127.0.0.1:4002",
			"http_proxy=http://127.0.0.1:4002",
			"NO_PROXY=127.0.0.1,localhost",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stdout, _, err := ts.Execute(context.Background(), []string{"sh", "-c", `echo "$HTTP_PROXY|$http_proxy|$NO_PROXY"`})
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	data, _ := io.ReadAll(stdout)
	want := "http://127.0.0.1:4002|http://127.0.0.1:4002|127.0.0.1,localhost"
	if strings.TrimSpace(string(data)) != want {
		t.Errorf("proxy env = %q, want %q", strings.TrimSpace(string(data)), want)
	}
}

// fakeSandbox records Wrap calls so tests can assert the S4 seam is applied
// before a shell starts (no-op provider must still be invoked at spawn).
type fakeSandbox struct {
	calls    int
	lastRoot string
	lastNet  bool
}

func (f *fakeSandbox) Wrap(_ *exec.Cmd, ws sandbox.WorkspaceView) error {
	f.calls++
	f.lastRoot = ws.RootPath
	f.lastNet = ws.NetworkOn
	return nil
}
func (f *fakeSandbox) Effective() sandbox.EffectiveState {
	return sandbox.EffectiveState{Provider: "fake"}
}

// The OS confinement provider must be applied at shell spawn with the policy's
// workspace view (plan Phase 2 seam).
func TestHostShellManager_AppliesSandboxProvider(t *testing.T) {
	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("NewHostShellManager: %v", err)
	}
	defer hm.Shutdown(context.Background())
	tmpDir, _ := os.MkdirTemp("", "shell-sandbox-*")
	defer os.RemoveAll(tmpDir)

	fake := &fakeSandbox{}
	if _, err := hm.GetOrCreate(context.Background(), "sbx-ws", tmpDir, WorkspacePolicy{
		NetworkOn: true,
		Sandbox:   fake,
	}); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("Wrap calls = %d, want 1", fake.calls)
	}
	if fake.lastRoot != tmpDir || !fake.lastNet {
		t.Errorf("Wrap view = (root=%q net=%v), want (%q, true)", fake.lastRoot, fake.lastNet, tmpDir)
	}
	// A nil provider (default) must still spawn fine.
	if _, err := hm.GetOrCreate(context.Background(), "sbx-ws2", tmpDir, WorkspacePolicy{}); err != nil {
		t.Fatalf("GetOrCreate without provider: %v", err)
	}
}

// Env-secret audit (plan Phase 4, pulled forward): an UNCONFIGURED allowlist
// must never pass the whole host environment into agent shells. Secrets and
// socket hatches must be absent; safe locale vars present.
func TestPrepareShellEnv_EmptyAllowlistDefaultsToSafeSet(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("SERVICE_CLIENT_SECRET", "super-secret")
	t.Setenv("TELEGRAM_BOT_TOKEN", "tok-secret")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("COLORTERM", "truecolor")

	tmpDir := t.TempDir()
	env := BuildSandboxEnv(tmpDir, nil, nil) // nil allowlist → safe default
	joined := strings.Join(env, "\n")
	for _, secret := range []string{"SSH_AUTH_SOCK", "SERVICE_CLIENT_SECRET", "TELEGRAM_BOT_TOKEN"} {
		if strings.Contains(joined, secret) {
			t.Errorf("secret %s leaked into shell env:\n%s", secret, joined)
		}
	}
	for _, safe := range []string{"LANG=en_US.UTF-8", "COLORTERM=truecolor", "HOME=" + tmpDir + "/.sandbox"} {
		if !strings.Contains(joined, safe) {
			t.Errorf("expected %q in env floor:\n%s", safe, joined)
		}
	}
}

// An explicit operator allowlist is still honored verbatim (never widened).
func TestPrepareShellEnv_ExplicitAllowlistHonored(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("LANG", "en_US.UTF-8")

	tmpDir := t.TempDir()
	env := BuildSandboxEnv(tmpDir, []string{"LANG"}, nil)
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "LANG=en_US.UTF-8") {
		t.Errorf("explicitly allowed var missing:\n%s", joined)
	}
	if strings.Contains(joined, "SSH_AUTH_SOCK") {
		t.Errorf("non-allowlisted secret leaked:\n%s", joined)
	}
}

// D8 dual-scope pooling: reaping an idle scope must not touch the workspace's
// other (freshly used) scope session — stale-scope shells are idle-reaped
// independently per composite key.
func TestHostShellManager_DualScopeIdleReapKeepsOtherScope(t *testing.T) {
	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("NewHostShellManager: %v", err)
	}
	defer hm.Shutdown(context.Background())
	tmpDir, _ := os.MkdirTemp("", "shell-dual-reap-*")
	defer os.RemoveAll(tmpDir)

	// Network-off session idles out quickly.
	if _, err := hm.GetOrCreate(context.Background(), "wsD", tmpDir, WorkspacePolicy{NetworkOn: false, IdleTimeout: 10 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)

	// Network-on session created now with no idle timeout (fresh use).
	kept, err := hm.GetOrCreate(context.Background(), "wsD", tmpDir, WorkspacePolicy{NetworkOn: true})
	if err != nil {
		t.Fatal(err)
	}
	hm.reap()

	views := hm.ListSessions()
	var sawOn, sawOff bool
	for _, v := range views {
		if v.WorkspaceID != "wsD" {
			continue
		}
		if v.NetworkOn {
			sawOn = true
		} else {
			sawOff = true
		}
	}
	if !sawOn {
		t.Error("the freshly used network-on scope must survive the idle reap")
	}
	if sawOff {
		t.Error("the idle network-off scope must be reaped (per-scope idle timeout)")
	}
	// The surviving session must still be usable.
	ts, err := hm.GetOrCreate(context.Background(), "wsD", tmpDir, WorkspacePolicy{NetworkOn: true})
	if err != nil {
		t.Fatal(err)
	}
	if ts != kept {
		t.Error("re-requesting the kept scope must return the same pooled session")
	}
}
