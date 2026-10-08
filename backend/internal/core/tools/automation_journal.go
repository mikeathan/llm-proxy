package tools

import (
	"context"
	"errors"
	"strings"

	"llm-proxy/models"
)

var (
	errJournalNotEnabled = errors.New("the journal is not enabled for this run")
	errJournalNoTarget   = errors.New("the journal needs a workspace and an automation name")
	errJournalEmpty      = errors.New("journal content is empty: write the full updated notes")
)

// JournalStore persists an automation's journal. *persistence.WorkspaceManager
// satisfies it; the interface is declared here so this package stays free of
// the persistence dependency.
type JournalStore interface {
	WriteJournal(workspaceID, automation, content string) error
}

// AutomationJournalTools lets an unattended run keep notes for its future runs.
type AutomationJournalTools struct {
	store JournalStore
}

// NewAutomationJournalTools returns the journal tool provider.
func NewAutomationJournalTools(store JournalStore) *AutomationJournalTools {
	return &AutomationJournalTools{store: store}
}

// AutomationJournalArgs are the automation_journal tool arguments.
type AutomationJournalArgs struct {
	Content string `json:"content"`
}

// Write replaces the running automation's journal. The target comes from the run
// context, never from arguments, so a run can only write its own journal; empty
// text is refused so a run cannot wipe its notes by accident. The stored text is
// sanitised and capped (models.SanitizeJournal).
func (j *AutomationJournalTools) Write(ctx context.Context, args AutomationJournalArgs) (any, error) {
	if !models.IsJournalRun(ctx) {
		return nil, errJournalNotEnabled
	}
	workspace, automation := models.GetWorkspaceID(ctx), models.GetTaskName(ctx)
	if workspace == "" || automation == "" {
		return nil, errJournalNoTarget
	}
	if strings.TrimSpace(args.Content) == "" {
		return nil, errJournalEmpty
	}
	saved := models.SanitizeJournal(args.Content)
	if err := j.store.WriteJournal(workspace, automation, saved); err != nil {
		return nil, err
	}
	chars := len([]rune(saved))
	return map[string]any{
		"saved":      true,
		"characters": chars,
		"truncated":  chars == models.MaxJournalChars && len([]rune(strings.TrimSpace(args.Content))) > chars,
	}, nil
}
