package persistence

import (
	"fmt"
	"os"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// journalDirName is the metadata subfolder holding per-automation learning
// journals. Like the seen ledgers it sits outside the agent's workspace jail:
// a run changes its journal only through the journal tool, which sanitises and
// caps what it writes.
const journalDirName = "journal"

func (m *WorkspaceManager) journalPath(workspaceID, automation string) string {
	return m.automationFilePath(workspaceID, journalDirName, automation, ".md")
}

// ReadJournal returns an automation's journal. A missing file is an empty
// journal, not an error.
func (m *WorkspaceManager) ReadJournal(workspaceID, automation string) (string, error) {
	data, err := os.ReadFile(m.journalPath(workspaceID, automation))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read journal: %w", err)
	}
	return string(data), nil
}

// WriteJournal replaces an automation's journal atomically with the sanitised,
// capped text (models.SanitizeJournal). Empty text clears the journal.
func (m *WorkspaceManager) WriteJournal(workspaceID, automation, content string) error {
	content = models.SanitizeJournal(content)
	if content == "" {
		return m.DeleteJournal(workspaceID, automation)
	}
	return storage.WriteAtomic(m.journalPath(workspaceID, automation), "journal-*.md.tmp", []byte(content), storage.ClassUserContent)
}

// DeleteJournal removes an automation's journal. A missing file is not an error.
func (m *WorkspaceManager) DeleteJournal(workspaceID, automation string) error {
	if err := os.Remove(m.journalPath(workspaceID, automation)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove journal: %w", err)
	}
	return nil
}
