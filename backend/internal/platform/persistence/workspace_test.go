package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// Task files are addressed by workspace-relative slash paths; every operation is
// contained in the workspace root (plan Phase 3): nested paths work, and neither
// traversal nor a symlink can reach outside the workspace.
func TestWorkspaceManager_TaskFileContainment(t *testing.T) {
	base := t.TempDir()
	resolver := storage.NewPathResolver(base, base, t.TempDir())
	mgr := NewWorkspaceManager(resolver)
	const wsID = "contained"
	wsDir := resolver.WorkspaceDir(wsID)

	t.Run("nested write, read and delete round-trip", func(t *testing.T) {
		if err := mgr.WriteTaskFile(wsID, "sub/task.md", "nested"); err != nil {
			t.Fatalf("WriteTaskFile: %v", err)
		}
		got, err := mgr.ReadTaskFile(wsID, "sub/task.md")
		if err != nil || got != "nested" {
			t.Fatalf("ReadTaskFile = %q, %v", got, err)
		}
		if err := mgr.DeleteTaskFile(wsID, "sub/task.md"); err != nil {
			t.Fatalf("DeleteTaskFile: %v", err)
		}
		if _, err := os.Stat(filepath.Join(wsDir, "sub", "task.md")); !os.IsNotExist(err) {
			t.Fatalf("file still present: %v", err)
		}
	})

	t.Run("a tree delete removes a folder and everything under it", func(t *testing.T) {
		for _, p := range []string{"tree/a.md", "tree/deep/b.md", "keep.md"} {
			if err := mgr.WriteTaskFile(wsID, p, "x"); err != nil {
				t.Fatalf("WriteTaskFile %s: %v", p, err)
			}
		}
		if err := mgr.DeleteTaskTree(context.Background(), wsID, "tree"); err != nil {
			t.Fatalf("DeleteTaskTree: %v", err)
		}
		if _, err := os.Stat(filepath.Join(wsDir, "tree")); !os.IsNotExist(err) {
			t.Fatalf("folder still present: %v", err)
		}
		if _, err := os.Stat(filepath.Join(wsDir, "keep.md")); err != nil {
			t.Fatalf("sibling removed: %v", err)
		}
	})

	t.Run("a tree delete stops on a cancelled context", func(t *testing.T) {
		if err := mgr.WriteTaskFile(wsID, "kept/a.md", "x"); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := mgr.DeleteTaskTree(ctx, wsID, "kept"); err == nil {
			t.Fatal("expected the cancelled context's error")
		}
		if _, err := os.Stat(filepath.Join(wsDir, "kept", "a.md")); err != nil {
			t.Fatalf("file removed despite cancellation: %v", err)
		}
	})

	t.Run("a missing file reads as empty", func(t *testing.T) {
		got, err := mgr.ReadTaskFile(wsID, "nope.md")
		if err != nil || got != "" {
			t.Fatalf("ReadTaskFile = %q, %v", got, err)
		}
	})

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(wsDir, "leak.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(wsDir, "out")); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		op   func() error
	}{
		{"read through an escaping file link", func() error { _, err := mgr.ReadTaskFile(wsID, "leak.md"); return err }},
		{"read through an escaping dir link", func() error { _, err := mgr.ReadTaskFile(wsID, "out/secret.md"); return err }},
		{"write through an escaping dir link", func() error { return mgr.WriteTaskFile(wsID, "out/new.md", "x") }},
		{"delete through an escaping dir link", func() error { return mgr.DeleteTaskFile(wsID, "out/secret.md") }},
		{"tree delete through an escaping dir link", func() error { return mgr.DeleteTaskTree(context.Background(), wsID, "out/secret.md") }},
		{"tree delete by traversal", func() error { return mgr.DeleteTaskTree(context.Background(), wsID, "../"+wsID) }},
		{"traversal", func() error { _, err := mgr.ReadTaskFile(wsID, "../escape.md"); return err }},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			if err := tc.op(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	t.Run("a tree delete removes a link inside the folder, never its outside target", func(t *testing.T) {
		if err := mgr.WriteTaskFile(wsID, "linked/a.md", "x"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(wsDir, "linked", "escape")); err != nil {
			t.Fatal(err)
		}
		if err := mgr.DeleteTaskTree(context.Background(), wsID, "linked"); err != nil {
			t.Fatalf("DeleteTaskTree: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(wsDir, "linked")); !os.IsNotExist(err) {
			t.Fatalf("folder still present: %v", err)
		}
	})

	if data, err := os.ReadFile(secret); err != nil || string(data) != "secret" {
		t.Fatalf("outside file changed: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "new.md")); !os.IsNotExist(err) {
		t.Fatal("a file was written outside the workspace")
	}
}

func TestWorkspaceManager_HeartbeatStatus(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))

	if got, err := mgr.ReadHeartbeatStatus("ws"); err != nil || got != nil {
		t.Fatalf("no status yet must read as nil, got %v, %v", got, err)
	}
	at := time.Date(2026, 10, 4, 14, 30, 0, 0, time.UTC)
	if err := mgr.WriteHeartbeatStatus("ws", models.HeartbeatStatus{At: at, Result: models.HeartbeatQuiet}); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ReadHeartbeatStatus("ws")
	if err != nil || got == nil || got.Result != models.HeartbeatQuiet || !got.At.Equal(at) {
		t.Fatalf("round-trip failed: %+v, %v", got, err)
	}

	path := filepath.Join(mgr.resolver.InternalDir("ws"), heartbeatStatusFile)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := mgr.ReadHeartbeatStatus("ws"); err == nil || got != nil {
		t.Errorf("a corrupt status must be reported, not guessed: %v, %v", got, err)
	}
}

func TestWorkspaceManager_SeenLedger(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))

	got, err := mgr.ReadSeen("ws", "nightly")
	if err != nil || len(got) != 0 {
		t.Fatalf("missing ledger must read as empty, got %v, %v", got, err)
	}

	at := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	want := models.SeenLedger{"https://a.com/x": {Title: "A", At: at}}
	if err := mgr.WriteSeen("ws", "nightly", want); err != nil {
		t.Fatal(err)
	}
	got, err = mgr.ReadSeen("ws", "nightly")
	if err != nil || got["https://a.com/x"].Title != "A" || !got["https://a.com/x"].At.Equal(at) {
		t.Fatalf("round-trip failed: %v, %v", got, err)
	}

	// Names that sanitise alike must not share a ledger.
	if other, _ := mgr.ReadSeen("ws", "nightly!"); len(other) != 0 {
		t.Errorf("distinct automation names shared a ledger: %v", other)
	}
	// A traversing name must stay inside the seen directory.
	if p := mgr.seenPath("ws", "../../evil"); !strings.HasPrefix(p, filepath.Join(mgr.resolver.InternalDir("ws"), seenDirName)+string(filepath.Separator)) {
		t.Errorf("seen path escaped its directory: %s", p)
	}
}

func TestWorkspaceManager_SeenLedgerCorruptIsQuarantined(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	path := mgr.seenPath("ws", "nightly")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.ReadSeen("ws", "nightly"); err == nil {
		t.Fatal("corrupt ledger must report an error")
	}
	// The unreadable file is set aside, so the next WriteSeen cannot silently
	// destroy it.
	if _, err := os.Stat(path + seenCorruptSuffix); err != nil {
		t.Errorf("corrupt ledger was not preserved: %v", err)
	}
	if got, err := mgr.ReadSeen("ws", "nightly"); err != nil || len(got) != 0 {
		t.Errorf("after quarantine the ledger must read as empty, got %v, %v", got, err)
	}
}

func TestWorkspaceManager_DeleteAutomationRunsRemovesSeenLedger(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	ledger := models.SeenLedger{"https://a.com/x": {Title: "A", At: time.Now()}}
	for _, name := range []string{"nightly", "weekly"} {
		if err := mgr.WriteSeen("ws", name, ledger); err != nil {
			t.Fatal(err)
		}
	}

	if err := mgr.DeleteAutomationRuns("ws", "nightly"); err != nil {
		t.Fatal(err)
	}

	if got, _ := mgr.ReadSeen("ws", "nightly"); len(got) != 0 {
		t.Errorf("deleted automation kept its seen ledger: %v", got)
	}
	if got, _ := mgr.ReadSeen("ws", "weekly"); len(got) != 1 {
		t.Errorf("another automation's ledger must survive, got %v", got)
	}
}

// newRunsTestManager returns a manager whose workspaces each have a content
// dir (what makes them a workspace) and the given persisted run history.
func newRunsTestManager(t *testing.T, histories map[string][]models.AutomationRun) *WorkspaceManager {
	t.Helper()
	base := t.TempDir()
	mgr := NewWorkspaceManager(storage.NewPathResolver(base, filepath.Join(base, "workspaces"), filepath.Join(base, "meta")))
	for id, history := range histories {
		if err := os.MkdirAll(mgr.resolver.WorkspaceDir(id), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := mgr.WriteState(id, &models.AgentState{History: history}); err != nil {
			t.Fatal(err)
		}
	}
	return mgr
}

func TestWorkspaceManager_RecentRuns(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return t0.Add(time.Duration(minutes) * time.Minute) }

	t.Run("merges workspaces oldest first and fills a missing workspace id", func(t *testing.T) {
		mgr := newRunsTestManager(t, map[string][]models.AutomationRun{
			"a": {{ID: "a1", WorkspaceID: "a", Timestamp: at(1)}, {ID: "a3", Timestamp: at(3)}},
			"b": {{ID: "b2", WorkspaceID: "b", Timestamp: at(2)}},
		})

		runs, err := mgr.RecentRuns(context.Background())
		if err != nil {
			t.Fatalf("RecentRuns: %v", err)
		}
		var ids []string
		for _, run := range runs {
			ids = append(ids, run.ID)
		}
		if strings.Join(ids, ",") != "a1,b2,a3" {
			t.Fatalf("want a1,b2,a3 (oldest first), got %v", ids)
		}
		if runs[2].WorkspaceID != "a" {
			t.Fatalf("a run stored without a workspace id must report its workspace, got %q", runs[2].WorkspaceID)
		}
	})

	t.Run("keeps only the newest MaxRecentRuns", func(t *testing.T) {
		histories := map[string][]models.AutomationRun{}
		for i := range MaxRecentRuns + 5 {
			id := fmt.Sprintf("ws%03d", i)
			histories[id] = []models.AutomationRun{{ID: id, Timestamp: at(i)}}
		}
		mgr := newRunsTestManager(t, histories)

		runs, err := mgr.RecentRuns(context.Background())
		if err != nil {
			t.Fatalf("RecentRuns: %v", err)
		}
		if len(runs) != MaxRecentRuns {
			t.Fatalf("want %d runs, got %d", MaxRecentRuns, len(runs))
		}
		if runs[0].ID != "ws005" || runs[len(runs)-1].ID != fmt.Sprintf("ws%03d", MaxRecentRuns+4) {
			t.Fatalf("want the newest runs kept, got first=%s last=%s", runs[0].ID, runs[len(runs)-1].ID)
		}
	})

	t.Run("reflects deletions with no extra bookkeeping", func(t *testing.T) {
		mgr := newRunsTestManager(t, map[string][]models.AutomationRun{
			"a": {{ID: "r1", AutomationName: "x", Timestamp: at(1)}, {ID: "r2", AutomationName: "y", Timestamp: at(2)}},
		})
		if err := mgr.DeleteAutomationRuns("a", "x"); err != nil {
			t.Fatalf("DeleteAutomationRuns: %v", err)
		}

		runs, err := mgr.RecentRuns(context.Background())
		if err != nil {
			t.Fatalf("RecentRuns: %v", err)
		}
		if len(runs) != 1 || runs[0].ID != "r2" {
			t.Fatalf("want only r2 after clearing x, got %+v", runs)
		}
	})

	t.Run("skips a workspace whose state cannot be read", func(t *testing.T) {
		mgr := newRunsTestManager(t, map[string][]models.AutomationRun{
			"good": {{ID: "g1", Timestamp: at(1)}},
			"bad":  nil,
		})
		if err := os.WriteFile(mgr.resolver.State("bad"), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}

		runs, err := mgr.RecentRuns(context.Background())
		if err != nil {
			t.Fatalf("one unreadable workspace must not fail the whole feed: %v", err)
		}
		if len(runs) != 1 || runs[0].ID != "g1" {
			t.Fatalf("want the readable workspace's runs, got %+v", runs)
		}
	})

	t.Run("stops on a cancelled context", func(t *testing.T) {
		mgr := newRunsTestManager(t, map[string][]models.AutomationRun{"a": {{ID: "a1"}}})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := mgr.RecentRuns(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	})
}
