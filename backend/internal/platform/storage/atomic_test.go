package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic_success(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.json")

	if err := WriteAtomic(dest, "test-*.tmp", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("dest file not created: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("content = %q, want %q", data, `{"ok":true}`)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file in dir, got %d: %v", len(entries), entries)
	}
}

func TestWriteAtomic_createsDir(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "sub", "deep", "test.json")

	if err := WriteAtomic(dest, "test-*.tmp", []byte("hello")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("dest file not created: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want %q", data, "hello")
	}
}

func TestWriteAtomic_noTempOnSuccess(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.txt")

	if err := WriteAtomic(dest, "out-*.tmp", []byte("data")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestWriteAtomic_failureLeavesNoDest(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "ro")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Make the target directory read-only so CreateTemp fails, exercising the
	// early error path. Restore permissions so t.TempDir cleanup succeeds.
	t.Cleanup(func() { os.Chmod(sub, 0o755) })
	if err := os.Chmod(sub, 0o500); err != nil {
		t.Fatalf("setup chmod: %v", err)
	}

	dest := filepath.Join(sub, "out.txt")
	err := WriteAtomic(dest, "out-*.tmp", []byte("data"))
	if err == nil {
		t.Fatal("expected error on read-only dir, got nil")
	}

	// The destination must not be created on failure.
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Errorf("dest file unexpectedly created on failure: %s", dest)
	}
}
