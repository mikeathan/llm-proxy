package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "template-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a dummy template
	content := `## Task: Test Playbook
**ID:** ` + "`test-id`" + `
**Category:** Test Category

This is a test content.
`
	err = os.WriteFile(filepath.Join(tempDir, "test.md"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	store := NewTemplateStore(tempDir)

	t.Run("List", func(t *testing.T) {
		list, err := store.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// The 10 shipped templates are extracted on first run alongside the
		// custom one (Phase 7 extract-on-first-run; never overwrite existing).
		if len(list) != 11 {
			t.Errorf("expected 11 templates (10 shipped + 1 custom), got %d", len(list))
		}
		found := false
		for _, tmpl := range list {
			if tmpl.ID == "test-id" {
				found = true
				if tmpl.Name != "Test Playbook" {
					t.Errorf("expected Name Test Playbook, got %s", tmpl.Name)
				}
			}
		}
		if !found {
			t.Error("custom test template not listed")
		}
	})

	t.Run("Get", func(t *testing.T) {
		tmpl, err := store.Get("test-id")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if tmpl.ID != "test-id" {
			t.Errorf("expected ID test-id, got %s", tmpl.ID)
		}
		if tmpl.Name != "Test Playbook" {
			t.Errorf("expected Name Test Playbook, got %s", tmpl.Name)
		}
		if tmpl.Content != content {
			t.Errorf("content mismatch")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := store.Get("non-existent")
		if err == nil {
			t.Errorf("expected error for non-existent template")
		}
	})
}

// The sandbox conformance-probe template is part of the shipped library (go:embed)
// and must stay discoverable: List/Get parse its metadata and content. Guards the
// embed + metadata contract so edits cannot silently drop it from the UI picker.
func TestShippedSandboxProbeTemplate(t *testing.T) {
	store := NewTemplateStore(t.TempDir()) // extractShipped copies the embedded set

	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tmpl := range list {
		if tmpl.ID == "sandbox-conformance-probe" {
			found = true
			if tmpl.Name != "Sandbox Conformance Probe" {
				t.Errorf("expected Name 'Sandbox Conformance Probe', got %q", tmpl.Name)
			}
			if tmpl.Category != "security" {
				t.Errorf("expected Category 'security', got %q", tmpl.Category)
			}
		}
	}
	if !found {
		t.Fatal("sandbox-conformance-probe template missing from the shipped set")
	}

	tmpl, err := store.Get("sandbox-conformance-probe")
	if err != nil {
		t.Fatalf("Get(sandbox-conformance-probe): %v", err)
	}
	for _, want := range []string{
		"Surface A", "Surface B", "Surface C", "Observed posture",
		// Guards the app-layer vs OS-layer split and the per-grant expectation
		// matrix (otherwise a PASS can be printed while the kernel jail is absent).
		"Expected results by grant", "OS-layer enforcement",
		// Guards the evidence-bound verdict: a skipped probe must not be reported as
		// an observed anomaly, and an unverified grant must not PASS.
		"Evidence rule", "Required probes", "INCOMPLETE",
		// Guards the four independent scopes and the private/LAN fetch probe.
		"internet_only", "LAN_HOST",
	} {
		if !strings.Contains(tmpl.Content, want) {
			t.Errorf("template content missing %q", want)
		}
	}
}
