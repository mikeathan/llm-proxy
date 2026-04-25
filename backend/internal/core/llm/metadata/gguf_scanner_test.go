package metadata_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/core/llm/metadata"
)

func TestGGUFScanner_Scan_MissingFile(t *testing.T) {
	scanner := metadata.NewGGUFScanner()
	ctx := context.Background()

	_, err := scanner.Scan(ctx, "/non/existent/file.gguf")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestGGUFScanner_Scan_ContextCancelled(t *testing.T) {
	scanner := metadata.NewGGUFScanner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	dir := t.TempDir()
	path := filepath.Join(dir, "test.gguf")
	_ = os.WriteFile(path, []byte("dummy data"), 0644)

	_, err := scanner.Scan(ctx, path)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestGGUFScanner_Scan_InvalidFile(t *testing.T) {
	scanner := metadata.NewGGUFScanner()
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.gguf")
	_ = os.WriteFile(path, []byte("not a gguf file"), 0644)

	_, err := scanner.Scan(ctx, path)
	if err == nil {
		t.Fatal("expected error for invalid gguf file, got nil")
	}
}

func TestGGUFScanner_Scan_RespectsTimeout(t *testing.T) {
	scanner := metadata.NewGGUFScanner()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(2 * time.Millisecond) // Ensure timeout expires

	_, err := scanner.Scan(ctx, "any.gguf")
	if err == nil {
		t.Fatal("expected error for timed out context, got nil")
	}
}
