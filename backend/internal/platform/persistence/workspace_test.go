package persistence

import (
	"testing"
	"time"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func TestWorkspaceManager_Sessions(t *testing.T) {
	// Setup
	tmpBase := t.TempDir()
	mgr := NewWorkspaceManager(storage.NewPathResolver(tmpBase))
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
	mgr := NewWorkspaceManager(storage.NewPathResolver(tmpBase))

	rel := mgr.GetRelativeWorkspacePath()
	if rel == "" {
		t.Error("expected non-empty relative path")
	}
}
