package storage

import (
	"os"
	"path/filepath"
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
		// The 8 shipped templates are extracted on first run alongside the
		// custom one (Phase 7 extract-on-first-run; never overwrite existing).
		if len(list) != 9 {
			t.Errorf("expected 9 templates (8 shipped + 1 custom), got %d", len(list))
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
