package automation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewRunDir_EmptyTask verifies that an empty task segment does not collapse
// the run directory layout. filepath.Join drops empty elements, which would
// otherwise place the run directory directly beneath the model dir and orphan
// it from every deletion path. The fallback keeps the {model}/{task} contract.
func TestNewRunDir_EmptyTask(t *testing.T) {
	parent := t.TempDir()

	rd, err := NewRunDir(parent, "ws-1", "", "deepseek-v4-flash-0731")
	if err != nil {
		t.Fatalf("NewRunDir: %v", err)
	}

	rel, err := filepath.Rel(parent, rd.Root)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 4 {
		t.Fatalf("run dir rel path = %q, want 4 segments ({ws}/{model}/{task}/{runDir})", rel)
	}
	if parts[0] != "ws-1" {
		t.Errorf("segment 0 = %q, want %q", parts[0], "ws-1")
	}
	if parts[1] != "deepseek-v4-flash-0731" {
		t.Errorf("segment 1 = %q, want %q", parts[1], "deepseek-v4-flash-0731")
	}
	if parts[2] != unknownTaskFallback {
		t.Errorf("segment 2 = %q, want fallback %q", parts[2], unknownTaskFallback)
	}

	if _, err := os.Stat(rd.Root); err != nil {
		t.Fatalf("run dir should exist: %v", err)
	}
}

// TestNewRunDir_NonEmptyTask keeps the task segment verbatim.
func TestNewRunDir_NonEmptyTask(t *testing.T) {
	parent := t.TempDir()

	rd, err := NewRunDir(parent, "ws-1", "automation-a", "model-x")
	if err != nil {
		t.Fatalf("NewRunDir: %v", err)
	}

	rel, err := filepath.Rel(parent, rd.Root)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 4 || parts[2] != "automation-a" {
		t.Fatalf("run dir rel path = %q, want task segment %q", rel, "automation-a")
	}
}
