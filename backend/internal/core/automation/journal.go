package automation

import "llm-proxy/internal/core/assistant/prompts"

// withJournal appends the automation's journal and the instruction to rewrite it
// to the task, when the automation keeps one. A journal that cannot be read is
// treated as empty (the run still goes ahead) and logged.
func (d *Dispatcher) withJournal(entry *AutomationEntry, task string) string {
	if !entry.Journal {
		return task
	}
	text, err := d.persistence.ReadJournal(entry.Workspace, entry.Name)
	if err != nil {
		d.logger.Warn("journal unreadable; treating as empty",
			"workspace", entry.Workspace, "automation", entry.Name, "error", err.Error())
		text = ""
	}
	return task + prompts.AutomationJournalBlock(text)
}
