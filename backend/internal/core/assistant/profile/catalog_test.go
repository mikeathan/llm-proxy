package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/internal/platform/logging"
)

func newCatalog(t *testing.T) (*Catalog, string) {
	t.Helper()
	dir := t.TempDir()
	c, err := NewCatalog(dir, logging.NewNopLogger())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return c, dir
}

func writeUser(t *testing.T, dir, id, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinsAreTheShippedSix(t *testing.T) {
	c, _ := newCatalog(t)
	var ids []string
	for _, p := range c.List() {
		ids = append(ids, p.ID)
		if p.Source != Builtin || p.Name == "" || p.Summary == "" || p.Instructions == "" {
			t.Errorf("built-in %s is incomplete: %+v", p.ID, p)
		}
	}
	if got, want := strings.Join(ids, ","), "builder,general,operator,research,task-runner,watcher"; got != want {
		t.Errorf("built-ins = %s, want %s", got, want)
	}
}

func TestDefaultsPerKind(t *testing.T) {
	c, _ := newCatalog(t)
	for kind, want := range map[Kind]string{KindAssistant: "general", KindAutomation: "task-runner"} {
		if got := c.Resolve(kind).ID; got != want {
			t.Errorf("default for %s = %s, want %s", kind, got, want)
		}
	}
	general := c.Resolve(KindAssistant)
	if !general.AnswerFirst || !general.Reminders || !general.GuidesMemorySaves() {
		t.Errorf("General must be the everyday assistant (answer-first, reminders, memory), got %+v", general)
	}
	hidden := strings.Join(general.ExcludedTools(), ",")
	for _, tool := range []string{"schedule_reminder", "internet_search", "read_file", "execute_terminal_command", "memory_update"} {
		if strings.Contains(hidden, tool) {
			t.Errorf("General is the everyday assistant and must keep %s, hidden = %s", tool, hidden)
		}
	}
	runner := c.Resolve(KindAutomation)
	if runner.AnswerFirst || runner.MayAsk || runner.Reminders {
		t.Errorf("Task runner is unattended and tool-first, got %+v", runner)
	}
}

func TestDefaultFor(t *testing.T) {
	if got := DefaultFor("general"); len(got) != 1 || got[0] != KindAssistant {
		t.Errorf("DefaultFor(general) = %v", got)
	}
	if got := DefaultFor("task-runner"); len(got) != 1 || got[0] != KindAutomation {
		t.Errorf("DefaultFor(task-runner) = %v", got)
	}
	if got := DefaultFor("research"); len(got) != 0 {
		t.Errorf("DefaultFor(research) = %v, want none", got)
	}
}

func TestResolvePrecedence(t *testing.T) {
	c, _ := newCatalog(t)
	// first known id wins: message, then conversation, then workspace; then the kind's default
	if got := c.Resolve(KindAssistant, "research", "builder").ID; got != "research" {
		t.Errorf("got %s, want research", got)
	}
	if got := c.Resolve(KindAssistant, "", "builder").ID; got != "builder" {
		t.Errorf("got %s, want builder", got)
	}
	if got := c.Resolve(KindAssistant, "gone", "also-gone").ID; got != "general" {
		t.Errorf("unknown ids fall back to the default, got %s", got)
	}
}

func TestUserProfilesAddAndOverride(t *testing.T) {
	c, dir := newCatalog(t)
	writeUser(t, dir, "poet", "---\nname: Poet\nsummary: Writes verse\nanswer_first: true\n---\nAnswer in verse.")
	writeUser(t, dir, "general", "---\nname: My General\nsummary: Mine\nanswer_first: true\nreminders: true\n---\nBe brief.")
	if p, ok := c.Get("poet"); !ok || p.Source != User || p.Instructions != "Answer in verse." {
		t.Errorf("poet = %+v, %v", p, ok)
	}
	if p := c.Resolve(KindAssistant, "general"); p.Name != "My General" || p.Source != User {
		t.Errorf("a user file with a built-in's id replaces it, got %+v", p)
	}
	if n := len(c.List()); n != 7 {
		t.Errorf("List has %d profiles, want 6 built-ins (one replaced) + 1 new = 7", n)
	}
}

func TestInvalidUserFilesAreSkippedNotFatal(t *testing.T) {
	c, dir := newCatalog(t)
	writeUser(t, dir, "broken", "no front matter")
	writeUser(t, dir, "Bad Name", "---\nname: X\n---\nbody")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("broken"); ok {
		t.Error("an invalid file must not become a profile")
	}
	if n := len(c.List()); n != 6 {
		t.Errorf("List has %d profiles, want the 6 built-ins", n)
	}
}

func TestOversizedUserFileIsRefused(t *testing.T) {
	c, dir := newCatalog(t)
	writeUser(t, dir, "huge", "---\nname: Huge\n---\n"+strings.Repeat("x", maxFileBytes+1))
	if _, ok := c.Get("huge"); ok {
		t.Error("a file over the size cap must be refused")
	}
}

// With no profiles folder at all (a mock, a bare catalog) nothing is read from the working directory.
func TestEmptyUserDirReadsNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("general.md", []byte("---\nname: Stray\n---\nnot a profile folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := NewCatalog("", logging.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := c.Get("general"); p.Name != "General" || p.Source != Builtin {
		t.Errorf("an empty userDir must not read ./general.md, got %+v", p)
	}
}

func TestMissingUserDirIsFine(t *testing.T) {
	c, err := NewCatalog(filepath.Join(t.TempDir(), "absent"), logging.NewNopLogger())
	if err != nil || len(c.List()) != 6 {
		t.Fatalf("NewCatalog with no profiles dir: %v, %d profiles", err, len(c.List()))
	}
}

func TestGetRejectsPathLikeIDs(t *testing.T) {
	c, _ := newCatalog(t)
	for _, id := range []string{"../etc/passwd", "a/b", "", "..", "Upper"} {
		if _, ok := c.Get(id); ok {
			t.Errorf("Get(%q) must not find anything", id)
		}
	}
}

// Each built-in states how it may save memory: the everyday assistant decides itself, task profiles save only what the
// user asks for, and the unattended quiet check saves nothing (its "already reported" state is the code-owned ledger).
func TestBuiltinMemorySaveModes(t *testing.T) {
	c, _ := newCatalog(t)
	want := map[string]SaveMode{
		"general": SaveModel, "task-runner": SaveModel, "research": SaveExplicit, "operator": SaveExplicit,
		"builder": SaveExplicit, "watcher": SaveOff,
	}
	for id, mode := range want {
		p, ok := c.Get(id)
		if !ok || p.MemorySave != mode {
			t.Errorf("%s: memory_save = %q (found %v), want %q", id, p.MemorySave, ok, mode)
		}
	}
}
