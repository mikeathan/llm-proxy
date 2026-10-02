package memory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"llm-proxy/internal/platform/storage"
)

// Operator notes are the operator's own standing instructions — "MEMORY.md" —
// injected at the top of every run's memory block, ahead of the agent's saved
// facts (see assistant/hot_memory.go). They are files, not rows, so they can be
// read and backed up like any other config, and they live OUTSIDE the agent's
// workspace jail (<metadata>/<workspace>/MEMORY.md and <root>/MEMORY.md): an
// agent cannot overwrite text that rides in every prompt. They are edited from
// the Memory panel, which goes through SetOperatorNotes.

// MaxOperatorNotesChars bounds one notes file. Notes are never clipped at
// injection time (the operator wrote them on purpose), so the bound is applied
// on write instead.
const MaxOperatorNotesChars = 6000

const notesFileName = "MEMORY.md"

var (
	ErrNotesTooLong          = errors.New("operator notes are too long")
	ErrNotesUnavailable      = errors.New("operator notes are not configured")
	ErrInvalidNotesWorkspace = errors.New("invalid workspace for operator notes")
)

// NotesScope selects which notes file is addressed.
type NotesScope string

const (
	NotesGlobal    NotesScope = "global"
	NotesWorkspace NotesScope = "workspace"
)

// OperatorNotes are the notes that apply to one workspace's runs.
type OperatorNotes struct {
	Global    string `json:"global"`
	Workspace string `json:"workspace"`
}

// Text joins the global notes then the workspace notes, trimmed; "" when neither has text.
func (n OperatorNotes) Text() string {
	parts := make([]string, 0, 2)
	for _, t := range []string{n.Global, n.Workspace} {
		if t = strings.TrimSpace(t); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}

// SetNotesLocations tells the store where the notes files live. Call once at
// startup, before the store is shared. Without it the notes feature is off:
// reads are empty and writes fail with ErrNotesUnavailable.
func (s *Store) SetNotesLocations(globalPath string, workspacePath func(workspaceID string) string) {
	s.notesGlobalPath = globalPath
	s.notesWorkspacePath = workspacePath
}

// OperatorNotes reads the global and the workspace notes. A missing file is
// empty notes, not an error.
func (s *Store) OperatorNotes(ctx context.Context, workspaceID string) (OperatorNotes, error) {
	var notes OperatorNotes
	var err error
	if notes.Global, err = readNotesFile(s.notesGlobalPath); err != nil {
		return OperatorNotes{}, err
	}
	if s.notesWorkspacePath == nil || !safeNotesWorkspace(workspaceID) {
		return notes, nil
	}
	if notes.Workspace, err = readNotesFile(s.notesWorkspacePath(workspaceID)); err != nil {
		return OperatorNotes{}, err
	}
	return notes, nil
}

// SetOperatorNotes replaces one notes file. Blank content removes the file.
// workspaceID is ignored for NotesGlobal.
func (s *Store) SetOperatorNotes(ctx context.Context, scope NotesScope, workspaceID, content string) error {
	path, err := s.notesPath(scope, workspaceID)
	if err != nil {
		return err
	}
	content = strings.TrimSpace(content)
	if len(content) > MaxOperatorNotesChars {
		return fmt.Errorf("%w: %d characters, limit %d", ErrNotesTooLong, len(content), MaxOperatorNotesChars)
	}
	if content == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove operator notes: %w", err)
		}
		return nil
	}
	if err := storage.WriteAtomic(path, notesFileName+".*", []byte(content+"\n"), storage.ClassData); err != nil {
		return fmt.Errorf("write operator notes: %w", err)
	}
	return nil
}

func (s *Store) notesPath(scope NotesScope, workspaceID string) (string, error) {
	switch scope {
	case NotesGlobal:
		if s.notesGlobalPath == "" {
			return "", ErrNotesUnavailable
		}
		return s.notesGlobalPath, nil
	case NotesWorkspace:
		if s.notesWorkspacePath == nil {
			return "", ErrNotesUnavailable
		}
		if !safeNotesWorkspace(workspaceID) {
			return "", ErrInvalidNotesWorkspace
		}
		return s.notesWorkspacePath(workspaceID), nil
	}
	return "", fmt.Errorf("unknown notes scope %q", scope)
}

// safeNotesWorkspace reports whether id can be used as a single path element.
func safeNotesWorkspace(id string) bool {
	return id != "" && id != "." && id != ".." && id == filepath.Base(id) && !strings.ContainsAny(id, `/\`)
}

func readNotesFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read operator notes: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
