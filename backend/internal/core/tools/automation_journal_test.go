package tools

import (
	"context"
	"strings"
	"testing"

	"llm-proxy/models"
)

type fakeJournalStore struct {
	workspace, automation, content string
	writes                         int
}

func (f *fakeJournalStore) WriteJournal(workspaceID, automation, content string) error {
	f.workspace, f.automation, f.content = workspaceID, automation, content
	f.writes++
	return nil
}

func journalRunContext() context.Context {
	ctx := models.WithWorkspaceID(context.Background(), "ws")
	ctx = models.WithTaskName(ctx, "nightly")
	return models.WithJournalRun(ctx)
}

func TestAutomationJournalTools_Write(t *testing.T) {
	t.Run("saves the sanitised text for the running automation", func(t *testing.T) {
		store := &fakeJournalStore{}
		res, err := NewAutomationJournalTools(store).Write(journalRunContext(), AutomationJournalArgs{Content: "## Worked\n- query A\x07"})
		if err != nil {
			t.Fatal(err)
		}
		if store.workspace != "ws" || store.automation != "nightly" || store.content != "## Worked\n- query A" {
			t.Errorf("stored %q/%q %q", store.workspace, store.automation, store.content)
		}
		if got := res.(map[string]any)["characters"]; got != len("## Worked\n- query A") {
			t.Errorf("reported characters = %v", got)
		}
	})

	t.Run("reports when the text was cut to the cap", func(t *testing.T) {
		res, err := NewAutomationJournalTools(&fakeJournalStore{}).Write(journalRunContext(), AutomationJournalArgs{Content: strings.Repeat("x", models.MaxJournalChars+10)})
		if err != nil {
			t.Fatal(err)
		}
		if res.(map[string]any)["truncated"] != true {
			t.Errorf("expected truncated=true, got %v", res)
		}
	})

	t.Run("refuses a run that did not enable the journal", func(t *testing.T) {
		store := &fakeJournalStore{}
		ctx := models.WithTaskName(models.WithWorkspaceID(context.Background(), "ws"), "nightly")
		if _, err := NewAutomationJournalTools(store).Write(ctx, AutomationJournalArgs{Content: "notes"}); err == nil || store.writes != 0 {
			t.Errorf("a non-journal run must be refused without writing: err=%v writes=%d", err, store.writes)
		}
	})

	t.Run("refuses empty text and a missing automation", func(t *testing.T) {
		store := &fakeJournalStore{}
		j := NewAutomationJournalTools(store)
		if _, err := j.Write(journalRunContext(), AutomationJournalArgs{Content: " \n"}); err == nil {
			t.Error("empty content must be refused so a run cannot wipe its journal by accident")
		}
		noName := models.WithJournalRun(models.WithWorkspaceID(context.Background(), "ws"))
		if _, err := j.Write(noName, AutomationJournalArgs{Content: "notes"}); err == nil {
			t.Error("a run without an automation name must be refused")
		}
		if store.writes != 0 {
			t.Errorf("writes = %d, want 0", store.writes)
		}
	})
}
