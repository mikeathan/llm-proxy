package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	shipped "llm-proxy/data/templates"
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
		// The shipped templates are extracted on first run alongside the custom
		// one (Phase 7 extract-on-first-run; never overwrite existing). Counted from
		// the embedded set so adding a template does not break this test.
		shippedEntries, err := shipped.FS.ReadDir(".")
		if err != nil {
			t.Fatal(err)
		}
		shippedCount := 0
		for _, e := range shippedEntries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				shippedCount++
			}
		}
		if want := shippedCount + 1; len(list) != want {
			t.Errorf("expected %d templates (%d shipped + 1 custom), got %d", want, shippedCount, len(list))
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

	store := NewTemplateStore(t.TempDir())
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

// A shipped template that was never edited follows upgrades; an edited one, or one with no record of what was
// seeded, is left exactly as the operator has it.
func TestTemplateStore_SyncRefreshesOnlyUneditedShippedTemplates(t *testing.T) {
	src := func(files map[string]string) fstest.MapFS {
		fsys := fstest.MapFS{}
		for name, body := range files {
			fsys[name] = &fstest.MapFile{Data: []byte(body)}
		}
		return fsys
	}
	read := func(t *testing.T, dir, name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	write := func(t *testing.T, dir, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("seeds missing files, then follows a new shipped version", func(t *testing.T) {
		dir := t.TempDir()
		(&TemplateStore{baseDir: dir}).syncTemplates(src(map[string]string{"a.md": "A1"}))
		if got := read(t, dir, "a.md"); got != "A1" {
			t.Fatalf("seeded = %q", got)
		}
		(&TemplateStore{baseDir: dir}).syncTemplates(src(map[string]string{"a.md": "A2"}))
		if got := read(t, dir, "a.md"); got != "A2" {
			t.Errorf("an unedited template did not follow the new version: %q", got)
		}
	})

	t.Run("an edited template stays, now and after later versions", func(t *testing.T) {
		dir := t.TempDir()
		store := &TemplateStore{baseDir: dir}
		store.syncTemplates(src(map[string]string{"b.md": "B1"}))
		write(t, dir, "b.md", "my edit")
		store.syncTemplates(src(map[string]string{"b.md": "B2"}))
		store.syncTemplates(src(map[string]string{"b.md": "B3"}))
		if got := read(t, dir, "b.md"); got != "my edit" {
			t.Errorf("the operator's edit was overwritten: %q", got)
		}
	})

	t.Run("a file with no record is left alone unless it already matches", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "old.md", "stale or edited")
		write(t, dir, "same.md", "S1")
		store := &TemplateStore{baseDir: dir}
		store.syncTemplates(src(map[string]string{"old.md": "O2", "same.md": "S1"}))
		if got := read(t, dir, "old.md"); got != "stale or edited" {
			t.Errorf("an unrecorded file was overwritten: %q", got)
		}
		// A file that matches the shipped text is adopted, so it follows the next version.
		store.syncTemplates(src(map[string]string{"old.md": "O2", "same.md": "S2"}))
		if got := read(t, dir, "same.md"); got != "S2" {
			t.Errorf("a file that matched the shipped text did not follow the next version: %q", got)
		}
	})

	t.Run("an unreadable record changes nothing", func(t *testing.T) {
		dir := t.TempDir()
		store := &TemplateStore{baseDir: dir}
		store.syncTemplates(src(map[string]string{"c.md": "C1"}))
		write(t, dir, shippedManifestFile, "{not json")
		store.syncTemplates(src(map[string]string{"c.md": "C2"}))
		if got := read(t, dir, "c.md"); got != "C1" {
			t.Errorf("a corrupt record caused an overwrite: %q", got)
		}
	})

	t.Run("the record is not listed as a template", func(t *testing.T) {
		dir := t.TempDir()
		store := &TemplateStore{baseDir: dir}
		store.syncTemplates(src(map[string]string{"d.md": "## Task: D\n**ID:** `d`\n**Category:** x\n"}))
		list, err := store.List()
		if err != nil || len(list) != 1 {
			t.Fatalf("List = %v, %v; want only d.md", list, err)
		}
	})
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
