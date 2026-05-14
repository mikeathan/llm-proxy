package shell

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHostShellManager_Execution(t *testing.T) {
	ctx := context.Background()
	hm, err := NewHostShellManager()
	if err != nil {
		t.Fatalf("failed to create HostShellManager: %v", err)
	}
	defer hm.Shutdown()

	tmpDir, _ := os.MkdirTemp("", "shell-test-*")
	defer os.RemoveAll(tmpDir)

	_ = os.WriteFile(filepath.Join(tmpDir, "hello.txt"), []byte("hello shell"), 0644)

	t.Run("Create and Execute", func(t *testing.T) {
		ts, err := hm.GetOrCreate(ctx, "test-ws", tmpDir, 0, nil, nil)
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
		ts, _ := hm.GetOrCreate(ctx, "test-ws", tmpDir, 0, nil, nil)
		
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
		_, _ = hm.GetOrCreate(ctx, "reap-ws", tmpDir, 10*time.Millisecond, nil, nil)
		time.Sleep(50 * time.Millisecond)
		hm.reap() // Trigger manual reap for fast test

		if _, ok := hm.sessions["reap-ws"]; ok {
			t.Errorf("expected session 'reap-ws' to be reaped due to inactivity")
		}
	})

	t.Run("Environment Inheritance", func(t *testing.T) {
		// Set a custom host variable
		os.Setenv("TEST_VAR_ANTIGRAVITY", "stabilized")
		defer os.Unsetenv("TEST_VAR_ANTIGRAVITY")

		ts, _ := hm.GetOrCreate(ctx, "env-ws", tmpDir, 0, []string{"TEST_VAR_ANTIGRAVITY", "HOME", "GOPATH", "TMPDIR"}, nil)
		
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
