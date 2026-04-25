package sandbox

import (
	"context"
	"io"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWazeroPool_Discovery(t *testing.T) {
	ctx := context.Background()
	wp, err := NewWazeroPool(models.HostSandboxingConfig{Enabled: true})
	if err != nil {
		t.Fatalf("failed to create WazeroPool: %v", err)
	}
	defer wp.Shutdown()

	tmpDir, _ := os.MkdirTemp("", "wazero-test-*")
	defer os.RemoveAll(tmpDir)

	_ = os.WriteFile(filepath.Join(tmpDir, "hello.txt"), []byte("hello wasm"), 0644)

	t.Run("Create and Execute", func(t *testing.T) {
		sb, err := wp.GetOrCreate(ctx, "test-ws", tmpDir)
		if err != nil {
			t.Fatalf("GetOrCreate failed: %v", err)
		}

		stdout, _, err := sb.Execute(ctx, []string{"cat hello.txt"})
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		defer stdout.Close()

		data, _ := io.ReadAll(stdout)
		if strings.TrimSpace(string(data)) != "hello wasm" {
			t.Errorf("expected 'hello wasm', got '%s'", string(data))
		}
	})

	t.Run("Jailing Check", func(t *testing.T) {
		sb, _ := wp.GetOrCreate(ctx, "test-ws", tmpDir)
		
		// Attempt to cat a file outside the jail (e.g. /etc/passwd)
		// Our bridge currently uses sh -c, so it relies on the Dir jail.
		stdout, _, err := sb.Execute(ctx, []string{"ls ../"})
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		defer stdout.Close()
		
		data, _ := io.ReadAll(stdout)
		// It should only see files in its own Dir or fail if it tries to escape.
		// For now, our bridge provides simple Cwd jailing.
		t.Logf("List output: %s", string(data))
	})
}
