package persistence

import (
	"fmt"
	"path/filepath"

	"llm-proxy/models"
)

// fixesFileName holds a workspace's fix records, beside the accepted findings and outside the agent's workspace jail:
// only the operator's endpoint writes it, so a run cannot mark its own findings fixed.
const fixesFileName = "fixes.json"

const fixesLabel = "fix records"

func (m *WorkspaceManager) fixesPath(workspaceID string) string {
	return filepath.Join(m.resolver.InternalDir(workspaceID), fixesFileName)
}

// ReadFixes returns the workspace's fix records. A missing file is none, not an error; an undecodable one is quarantined
// as *.corrupt and reported as an error. The file goes with the workspace when it is deleted.
func (m *WorkspaceManager) ReadFixes(workspaceID string) ([]models.FixRecord, error) {
	return readJSONFile[[]models.FixRecord](m.fixesPath(workspaceID), fixesLabel)
}

// WriteFixes replaces the workspace's fix records atomically.
func (m *WorkspaceManager) WriteFixes(workspaceID string, records []models.FixRecord) error {
	if err := writeJSONFile(m.fixesPath(workspaceID), "fixes-*.json.tmp", fixesLabel, records); err != nil {
		return fmt.Errorf("failed to save the fix records: %w", err)
	}
	return nil
}
