package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// notesStore returns a store whose operator notes live under a temp dir laid out
// like production: <meta>/<workspace>/MEMORY.md and <root>/MEMORY.md.
func notesStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	s := newTestStore(t)
	s.SetNotesLocations(filepath.Join(root, "MEMORY.md"), func(ws string) string {
		return filepath.Join(root, "meta", ws, "MEMORY.md")
	})
	return s, root
}

func TestOperatorNotes_RoundTripPerScopeAndWorkspace(t *testing.T) {
	s, _ := notesStore(t)
	ctx := context.Background()
	if err := s.SetOperatorNotes(ctx, NotesWorkspace, "ws-1", "Always use tabs."); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOperatorNotes(ctx, NotesWorkspace, "ws-2", "Use spaces."); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOperatorNotes(ctx, NotesGlobal, "", "Reply in British English."); err != nil {
		t.Fatal(err)
	}

	got, err := s.OperatorNotes(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace != "Always use tabs." || got.Global != "Reply in British English." {
		t.Errorf("ws-1 notes = %+v", got)
	}
	other, _ := s.OperatorNotes(ctx, "ws-2")
	if other.Workspace != "Use spaces." {
		t.Errorf("notes must be per workspace, ws-2 got %q", other.Workspace)
	}
}

func TestOperatorNotes_MissingFileIsEmptyNotAnError(t *testing.T) {
	s, _ := notesStore(t)
	got, err := s.OperatorNotes(context.Background(), "ws-1")
	if err != nil || got != (OperatorNotes{}) {
		t.Errorf("got %+v, %v; want empty notes and no error", got, err)
	}
}

func TestOperatorNotes_TooLongIsRefusedAndKeepsTheOldText(t *testing.T) {
	s, _ := notesStore(t)
	ctx := context.Background()
	if err := s.SetOperatorNotes(ctx, NotesWorkspace, "ws-1", "keep me"); err != nil {
		t.Fatal(err)
	}
	err := s.SetOperatorNotes(ctx, NotesWorkspace, "ws-1", strings.Repeat("x", MaxOperatorNotesChars+1))
	if !errors.Is(err, ErrNotesTooLong) {
		t.Fatalf("err = %v, want ErrNotesTooLong", err)
	}
	if got, _ := s.OperatorNotes(ctx, "ws-1"); got.Workspace != "keep me" {
		t.Errorf("a refused write must not change the file, got %q", got.Workspace)
	}
	if err := s.SetOperatorNotes(ctx, NotesWorkspace, "ws-1", strings.Repeat("x", MaxOperatorNotesChars)); err != nil {
		t.Errorf("exactly the limit must be accepted: %v", err)
	}
}

func TestOperatorNotes_EmptyContentRemovesTheFile(t *testing.T) {
	s, root := notesStore(t)
	ctx := context.Background()
	s.SetOperatorNotes(ctx, NotesWorkspace, "ws-1", "something")
	path := filepath.Join(root, "meta", "ws-1", "MEMORY.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if err := s.SetOperatorNotes(ctx, NotesWorkspace, "ws-1", "  \n "); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("blank notes should remove the file, stat err = %v", err)
	}
}

// The workspace id becomes a path element: anything that could leave the
// workspace's own folder is refused before touching the filesystem.
func TestOperatorNotes_RejectsUnsafeWorkspaceIDs(t *testing.T) {
	s, root := notesStore(t)
	for _, ws := range []string{"", "..", ".", "../x", "a/b", `a\b`} {
		if err := s.SetOperatorNotes(context.Background(), NotesWorkspace, ws, "x"); !errors.Is(err, ErrInvalidNotesWorkspace) {
			t.Errorf("workspace %q: err = %v, want ErrInvalidNotesWorkspace", ws, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "x")); err == nil {
		t.Error("an unsafe id must not create anything outside the meta folder")
	}
}

func TestOperatorNotes_UnavailableWhenNoLocationsAreConfigured(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if got, err := s.OperatorNotes(ctx, "ws-1"); err != nil || got != (OperatorNotes{}) {
		t.Errorf("reading without locations must be empty and quiet, got %+v, %v", got, err)
	}
	if err := s.SetOperatorNotes(ctx, NotesWorkspace, "ws-1", "x"); !errors.Is(err, ErrNotesUnavailable) {
		t.Errorf("err = %v, want ErrNotesUnavailable", err)
	}
}

func TestOperatorNotes_TextJoinsGlobalThenWorkspace(t *testing.T) {
	cases := []struct {
		name string
		in   OperatorNotes
		want string
	}{
		{"both", OperatorNotes{Global: "G", Workspace: "W"}, "G\n\nW"},
		{"global only", OperatorNotes{Global: " G \n"}, "G"},
		{"workspace only", OperatorNotes{Workspace: "W"}, "W"},
		{"neither", OperatorNotes{Global: "  ", Workspace: ""}, ""},
	}
	for _, c := range cases {
		if got := c.in.Text(); got != c.want {
			t.Errorf("%s: Text() = %q, want %q", c.name, got, c.want)
		}
	}
}
