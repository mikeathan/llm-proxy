package rundir

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

// TestRunMetaNetworkScopeAudit verifies the resolved per-run network scope is
// recorded in run-meta.json on both the success and the error writer path
// (sandboxing plan §4.4 audit trail — an unattended run that lost network is
// explainable at a glance).
func TestRunMetaNetworkScopeAudit(t *testing.T) {
	parent := t.TempDir()
	rd, err := NewRunDir(parent, "ws-1", "auto", "model-x")
	if err != nil {
		t.Fatal(err)
	}
	if err := rd.WriteMeta(RunMeta{
		Model:        "model-x",
		Task:         "auto",
		DurationMs:   12,
		NetworkScope: "lan",
	}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	data, err := os.ReadFile(rd.MetaPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"network_scope": "lan"`) {
		t.Errorf("run-meta.json must record the resolved network scope:\n%s", data)
	}

	// Error-path writer records it too, and an inherit run omits the key
	// (empty = inherit, matching the omitempty contract).
	if err := rd.WriteMeta(RunMeta{Model: "m", Task: "t", NetworkScope: ""}); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(rd.MetaPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "network_scope") {
		t.Errorf("inherit (empty) scope must be omitted from run-meta.json:\n%s", data)
	}
}
