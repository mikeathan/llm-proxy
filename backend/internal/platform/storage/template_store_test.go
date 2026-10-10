package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	shipped "llm-proxy/data/templates"
)

func TestTemplateStore(t *testing.T) {
	content := `## Task: Test Playbook
**ID:** ` + "`test-id`" + `
**Category:** Test Category

This is a test content.
`
	store := NewTemplateStore(fstest.MapFS{
		"test.md":         {Data: []byte(content)},
		"notes.txt":       {Data: []byte("not a playbook")},
		"nested/inner.md": {Data: []byte("## Task: Nested\n**ID:** `nested`\n**Category:** x\n")},
	})

	t.Run("List serves only the top-level markdown files of the source", func(t *testing.T) {
		list, err := store.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(list) != 1 || list[0].ID != "test-id" {
			t.Fatalf("List = %+v, want only test-id", list)
		}
		if list[0].Name != "Test Playbook" || list[0].Category != "Test Category" {
			t.Errorf("metadata = %+v", list[0])
		}
	})

	t.Run("an empty library lists as an empty slice, not null", func(t *testing.T) {
		list, err := NewTemplateStore(fstest.MapFS{}).List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if list == nil || len(list) != 0 {
			t.Errorf("List = %#v, want an empty non-nil slice", list)
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
		if _, err := store.Get("non-existent"); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Get(non-existent) = %v, want os.ErrNotExist", err)
		}
	})
}

// The library is exactly the embedded shipped set: nothing is copied to disk first, so a new binary serves its own
// playbooks with no refresh step.
func TestTemplateStore_ServesTheShippedSetDirectly(t *testing.T) {
	list, err := NewTemplateStore(shipped.FS).List()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := shipped.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			want++
		}
	}
	if len(list) != want {
		t.Errorf("listed %d templates, the embedded set holds %d", len(list), want)
	}
}

// The sandbox conformance-probe template is part of the shipped library (go:embed)
// and must stay discoverable: List/Get parse its metadata and content. Guards the
// embed + metadata contract so edits cannot silently drop it from the UI picker.
func TestShippedSandboxProbeTemplate(t *testing.T) {
	store := NewTemplateStore(shipped.FS)

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

// Every shipped template must have real content: a template emptied by a bad
// edit still parses as a file and would be handed to users as a blank task.
func TestShippedTemplatesAreNotEmpty(t *testing.T) {
	entries, err := shipped.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := shipped.FS.ReadFile(e.Name())
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		if len(strings.TrimSpace(string(data))) < minShippedTemplateBytes || !strings.Contains(string(data), "**ID:**") {
			t.Errorf("%s: shipped template is empty or has no **ID:** header (%d bytes)", e.Name(), len(data))
		}
	}
}

const minShippedTemplateBytes = 100

// The memory guide and the memory templates carry the same three prompts (store,
// recall, A/B), in that order. A verification or an experiment is only valid if what
// the operator runs is what was designed (the A/B needs both automations to run
// identical text), so each template must contain its guide prompt verbatim.
func TestMemoryTemplatesMatchTheGuidePrompts(t *testing.T) {
	guide, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "guides", "memory-testing.md"))
	if err != nil {
		t.Fatal(err)
	}
	const fence = "```markdown\n"
	var prompts []string
	rest := string(guide)
	for {
		start := strings.Index(rest, fence)
		if start < 0 {
			break
		}
		rest = rest[start+len(fence):]
		end := strings.Index(rest, "```")
		if end < 0 {
			t.Fatal("unterminated ```markdown block in the guide")
		}
		prompts = append(prompts, strings.TrimRight(rest[:end], "\n"))
		rest = rest[end+3:]
	}
	ids := []string{"memory-store-test", "memory-recall-test", "memory-ab-test"}
	if len(prompts) != len(ids) {
		t.Fatalf("the guide must hold exactly %d prompts (store, recall, A/B), found %d", len(ids), len(prompts))
	}

	store := NewTemplateStore(shipped.FS)
	for i, id := range ids {
		tmpl, err := store.Get(id)
		if err != nil {
			t.Fatalf("Get(%s): %v", id, err)
		}
		if !strings.Contains(tmpl.Content, prompts[i]) {
			t.Errorf("%s does not contain the guide's prompt %d verbatim; update both together", id, i+1)
		}
		if tmpl.Category != "memory" {
			t.Errorf("%s category = %q, want memory", id, tmpl.Category)
		}
	}
}

// The news brief learns between runs through its Memory section; the saved list must be the table's rows, or the next run
// reports an item twice (seen in a live run on 2026-10-05).
func TestShippedNewsBrief_MemorySectionKeepsTheSavedListEqualToTheTable(t *testing.T) {
	data, err := shipped.FS.ReadFile("llm_ai_release_brief.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"`memory_search`", "`memory_update`", "exactly the rows of your table", "BEFORE you send"} {
		if !strings.Contains(text, want) {
			t.Errorf("the brief's memory section lost %q", want)
		}
	}
}
