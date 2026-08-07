package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func TestWorkspaceManager_Sessions(t *testing.T) {
	// Setup
	tmpBase := t.TempDir()
	mgr := NewWorkspaceManager(storage.NewPathResolver(tmpBase, tmpBase, tmpBase))
	workspaceID := "test-workspace"

	t.Run("Write and Read Session", func(t *testing.T) {
		session := &models.AssistantSession{
			ID:             "session-1",
			WorkspaceID:    workspaceID,
			ContextVersion: "1.0",
			Timezone:       "UTC",
			History: []models.Message{
				{Role: models.UserRole, Content: "Hello"},
				{Role: models.AssistantRole, Content: "Hi there!"},
			},
		}

		err := mgr.WriteSession(workspaceID, session)
		if err != nil {
			t.Fatalf("failed to write session: %v", err)
		}

		// Read it back
		read, err := mgr.ReadSession(workspaceID, "session-1")
		if err != nil {
			t.Fatalf("failed to read session: %v", err)
		}

		if read == nil {
			t.Fatal("expected session to be found")
		}

		if read.ID != session.ID {
			t.Errorf("expected ID %s, got %s", session.ID, read.ID)
		}
		if len(read.History) != 2 {
			t.Errorf("expected 2 messages, got %d", len(read.History))
		}
		if read.History[1].Content != "Hi there!" {
			t.Errorf("expected content 'Hi there!', got '%s'", read.History[1].Content)
		}
		if read.UpdatedAt.IsZero() {
			t.Error("expected UpdatedAt to be set")
		}
	})

	t.Run("List Sessions", func(t *testing.T) {
		// Create a second session
		session2 := &models.AssistantSession{
			ID:          "session-2",
			WorkspaceID: workspaceID,
			History: []models.Message{
				{Role: models.UserRole, Content: "Another chat"},
			},
		}
		// Artificial delay to ensure different UpdatedAt
		time.Sleep(10 * time.Millisecond)
		if err := mgr.WriteSession(workspaceID, session2); err != nil {
			t.Fatalf("failed to write second session: %v", err)
		}

		briefs, err := mgr.ListSessions(workspaceID)
		if err != nil {
			t.Fatalf("failed to list sessions: %v", err)
		}

		if len(briefs) != 2 {
			t.Fatalf("expected 2 sessions, got %d", len(briefs))
		}

		// Should be sorted by UpdatedAt descending, so session-2 first
		if briefs[0].ID != "session-2" {
			t.Errorf("expected first session to be session-2, got %s", briefs[0].ID)
		}
		if briefs[0].Snippet != "Another chat" {
			t.Errorf("expected snippet 'Another chat', got '%s'", briefs[0].Snippet)
		}
		if briefs[1].ID != "session-1" {
			t.Errorf("expected second session to be session-1, got %s", briefs[1].ID)
		}
	})

	t.Run("Session Not Found", func(t *testing.T) {
		read, err := mgr.ReadSession(workspaceID, "non-existent")
		if err != nil {
			t.Errorf("expected no error for non-existent session, got %v", err)
		}
		if read != nil {
			t.Error("expected nil for non-existent session")
		}
	})

	t.Run("Empty Workspace List", func(t *testing.T) {
		briefs, err := mgr.ListSessions("empty-workspace")
		if err != nil {
			t.Errorf("expected no error for empty workspace, got %v", err)
		}
		if len(briefs) != 0 {
			t.Errorf("expected 0 sessions, got %d", len(briefs))
		}
	})
}

func TestWorkspaceManager_Paths(t *testing.T) {
	tmpBase := t.TempDir()
	mgr := NewWorkspaceManager(storage.NewPathResolver(tmpBase, tmpBase, tmpBase))

	rel := mgr.GetRelativeWorkspacePath()
	if rel == "" {
		t.Error("expected non-empty relative path")
	}
}

// TestWorkspaceManager_DeleteWorkspace_RemovesAllLocations verifies that
// deleting a workspace removes every on-disk location it owns: the user content
// dir, the full per-workspace metadata dir (config.yaml, state.json, .lock,
// process.log, sessions/), and the automation runs tree.
func TestWorkspaceManager_DeleteWorkspace_RemovesAllLocations(t *testing.T) {
	tmpWorkspaces := t.TempDir()
	tmpMetadata := t.TempDir()
	resolver := storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpMetadata)
	mgr := NewWorkspaceManager(resolver)
	wsID := "delete-me"

	// Seed user content.
	if err := mgr.WriteTaskFile(wsID, "notes.txt", "hello"); err != nil {
		t.Fatalf("WriteTaskFile: %v", err)
	}

	// Seed metadata: config, state, a session, a lock file and a process log.
	if err := mgr.WriteConfig(wsID, &models.WorkspaceConfig{Model: "m"}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	if err := mgr.WriteState(wsID, &models.AgentState{NextRunAt: time.Now()}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	if err := mgr.WriteSession(wsID, &models.AssistantSession{
		ID:          "s1",
		WorkspaceID: wsID,
		History:     []models.Message{{Role: models.UserRole, Content: "hi"}},
	}); err != nil {
		t.Fatalf("WriteSession: %v", err)
	}
	if err := os.WriteFile(resolver.ProcessLog(wsID), []byte("log"), 0600); err != nil {
		t.Fatalf("write process.log: %v", err)
	}
	if err := os.WriteFile(resolver.Lock(wsID), nil, 0600); err != nil {
		t.Fatalf("write .lock: %v", err)
	}

	// Seed an automation runs tree.
	if err := os.MkdirAll(filepath.Join(resolver.WorkspaceRunsDir(wsID), "m", "task", "20260815T120000Z_aaaa"), 0755); err != nil {
		t.Fatalf("seed run dir: %v", err)
	}

	// Every location must exist before the delete.
	for _, path := range []string{
		resolver.WorkspaceDir(wsID),
		resolver.InternalDir(wsID),
		resolver.WorkspaceRunsDir(wsID),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("precondition: %s should exist: %v", path, err)
		}
	}

	if err := mgr.DeleteWorkspace(wsID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	// Every location must be gone afterwards.
	for _, path := range []string{
		resolver.WorkspaceDir(wsID),
		resolver.InternalDir(wsID),
		resolver.WorkspaceRunsDir(wsID),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, err=%v", path, err)
		}
	}
}

// seedRunDir creates a run directory with a marker file so deletion tests can
// assert on real on-disk removal.
func seedRunDir(t *testing.T, base, model, task string) string {
	t.Helper()
	dir := filepath.Join(base, model, task, "20260815T120000Z_aaaa")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("seed run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("{}\n"), 0644); err != nil {
		t.Fatalf("seed events.jsonl: %v", err)
	}
	return dir
}

func TestWorkspaceManager_DeleteSession_RemovesRunDirs(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := NewWorkspaceManager(resolver)
	wsID := "ws-1"

	if err := mgr.WriteSession(wsID, &models.AssistantSession{
		ID: "conv_1", WorkspaceID: wsID,
		History: []models.Message{{Role: models.UserRole, Content: "hi"}},
	}); err != nil {
		t.Fatalf("WriteSession: %v", err)
	}

	runToDelete := seedRunDir(t, resolver.WorkspaceRunsDir(wsID), "m1", "conv_1")
	runToKeep := seedRunDir(t, resolver.WorkspaceRunsDir(wsID), "m1", "conv_2")

	if err := mgr.DeleteSession(wsID, "conv_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if _, err := os.Stat(runToDelete); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed, err=%v", runToDelete, err)
	}
	if _, err := os.Stat(runToKeep); err != nil {
		t.Errorf("expected unrelated run dir %s to remain, err=%v", runToKeep, err)
	}
}

func TestWorkspaceManager_DeleteAllSessions_RemovesRunDirs(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := NewWorkspaceManager(resolver)
	wsID := "ws-1"

	for _, id := range []string{"conv_1", "conv_2"} {
		if err := mgr.WriteSession(wsID, &models.AssistantSession{
			ID: id, WorkspaceID: wsID,
			History: []models.Message{{Role: models.UserRole, Content: "hi"}},
		}); err != nil {
			t.Fatalf("WriteSession %s: %v", id, err)
		}
	}

	runA := seedRunDir(t, resolver.WorkspaceRunsDir(wsID), "m1", "conv_1")
	runB := seedRunDir(t, resolver.WorkspaceRunsDir(wsID), "m1", "conv_2")
	// An automation task dir is not a session ID and must survive.
	runAuto := seedRunDir(t, resolver.WorkspaceRunsDir(wsID), "m1", "automation-x")

	if err := mgr.DeleteAllSessions(wsID); err != nil {
		t.Fatalf("DeleteAllSessions: %v", err)
	}

	for _, path := range []string{runA, runB} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, err=%v", path, err)
		}
	}
	if _, err := os.Stat(runAuto); err != nil {
		t.Errorf("expected automation run dir %s to remain, err=%v", runAuto, err)
	}
}

func TestFirstUserSnippet(t *testing.T) {
	long := "this is a very long user prompt that should be truncated because it exceeds the eighty character limit for the session list preview"
	cases := []struct {
		name string
		hist []models.Message
		want string
	}{
		{
			name: "first user message wins",
			hist: []models.Message{
				{Role: models.AssistantRole, Content: "previous reply"},
				{Role: models.UserRole, Content: "actual question"},
				{Role: models.AssistantRole, Content: "answer"},
			},
			want: "actual question",
		},
		{
			name: "falls back to first non-empty when no user role",
			hist: []models.Message{
				{Role: models.AssistantRole, Content: "only reply"},
			},
			want: "only reply",
		},
		{
			name: "truncates long content",
			hist: []models.Message{{Role: models.UserRole, Content: long}},
			want: long[:77] + "...",
		},
		{
			name: "empty history",
			hist: []models.Message{},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstUserSnippet(c.hist); got != c.want {
				t.Errorf("firstUserSnippet() = %q, want %q", got, c.want)
			}
		})
	}
}
