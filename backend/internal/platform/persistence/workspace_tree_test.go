package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func seedTree(t *testing.T, dir string, files []string, dirs []string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func entryMap(tree models.WorkspaceTree) map[string]models.TreeEntry {
	m := make(map[string]models.TreeEntry, len(tree.Entries))
	for _, e := range tree.Entries {
		m[e.Path] = e
	}
	return m
}

func TestWorkspaceManager_ListTree(t *testing.T) {
	base := t.TempDir()
	resolver := storage.NewPathResolver(base, base, t.TempDir())
	mgr := NewWorkspaceManager(resolver)
	const wsID = "tree"
	wsDir := resolver.WorkspaceDir(wsID)
	seedTree(t, wsDir,
		[]string{
			"AGENTS.md", "docs/plan.md", "docs/deep/notes.md", "docs/config.yaml",
			models.ConfigFilename, models.StateFilename, ".hidden.md", "docs/.secret.md",
			".sandbox/run.log", "sessions/old.json", "node_modules/pkg/index.js", "app/node_modules/x.js",
		},
		[]string{"empty"},
	)
	outside := t.TempDir()
	seedTree(t, outside, []string{"secret.md"}, nil)
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(wsDir, "leak.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("docs/plan.md", filepath.Join(wsDir, "plan-link.md")); err != nil {
		t.Fatal(err)
	}

	tree, err := mgr.ListTree(wsID)
	if err != nil {
		t.Fatalf("ListTree: %v", err)
	}
	got := entryMap(tree)

	want := map[string]models.TreeEntryType{
		"AGENTS.md":          models.TreeEntryFile,
		"docs":               models.TreeEntryDir,
		"docs/plan.md":       models.TreeEntryFile,
		"docs/deep":          models.TreeEntryDir,
		"docs/deep/notes.md": models.TreeEntryFile,
		"docs/config.yaml":   models.TreeEntryFile, // reserved names are reserved at the root only
		"empty":              models.TreeEntryDir,  // empty directories render
		"node_modules":       models.TreeEntryDir,
		"app":                models.TreeEntryDir,
		"app/node_modules":   models.TreeEntryDir,
		"plan-link.md":       models.TreeEntryFile, // a link that stays inside is a file
	}
	for p, typ := range want {
		if e, ok := got[p]; !ok || e.Type != typ {
			t.Errorf("entry %q = %+v, want type %q", p, e, typ)
		}
	}
	for _, absent := range []string{
		models.ConfigFilename, models.StateFilename, ".hidden.md", "docs/.secret.md", ".sandbox",
		".sandbox/run.log", "sessions", "sessions/old.json", "node_modules/pkg", "node_modules/pkg/index.js",
		"app/node_modules/x.js", "leak.md",
	} {
		if _, ok := got[absent]; ok {
			t.Errorf("entry %q should not be listed", absent)
		}
	}
	for _, heavy := range []string{"node_modules", "app/node_modules"} {
		if !got[heavy].Collapsed {
			t.Errorf("%q should be marked collapsed", heavy)
		}
	}
	if len(tree.Entries) != len(want) {
		t.Errorf("got %d entries, want %d: %+v", len(tree.Entries), len(want), tree.Entries)
	}
	for i := 1; i < len(tree.Entries); i++ {
		if tree.Entries[i-1].Path >= tree.Entries[i].Path {
			t.Fatalf("entries not sorted: %q before %q", tree.Entries[i-1].Path, tree.Entries[i].Path)
		}
	}
	if tree.Truncated {
		t.Error("tree should not be truncated")
	}
}

// The cap applies breadth-first, so truncation drops the deepest entries —
// never a whole top-level folder that happens to sort late.
func TestWorkspaceManager_ListTreeBreadthFirstCap(t *testing.T) {
	base := t.TempDir()
	resolver := storage.NewPathResolver(base, base, t.TempDir())
	const wsID = "capped"
	seedTree(t, resolver.WorkspaceDir(wsID), []string{"a/1/2/3/deep.md", "a/top.md", "z/late.md"}, nil)

	root, err := os.OpenRoot(resolver.WorkspaceDir(wsID))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	tree, err := listTree(root, 5)
	if err != nil {
		t.Fatalf("listTree: %v", err)
	}
	got := entryMap(tree)
	if !tree.Truncated || len(tree.Entries) != 5 {
		t.Fatalf("truncated=%v with %d entries, want true with 5", tree.Truncated, len(tree.Entries))
	}
	for _, p := range []string{"a", "z", "a/1", "a/top.md", "z/late.md"} {
		if _, ok := got[p]; !ok {
			t.Errorf("shallow entry %q missing", p)
		}
	}
}

func TestWorkspaceManager_ListTreeMissingWorkspace(t *testing.T) {
	base := t.TempDir()
	mgr := NewWorkspaceManager(storage.NewPathResolver(base, base, t.TempDir()))
	tree, err := mgr.ListTree("absent")
	if err != nil || len(tree.Entries) != 0 || tree.Truncated {
		t.Fatalf("ListTree(absent) = %+v, %v; want empty", tree, err)
	}
}
