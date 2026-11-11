package persistence

import (
	"fmt"
	"path/filepath"
	"sync"

	"llm-proxy/models"
)

// lifeFileName holds a workspace's finding life records, beside the fix records and outside the agent's workspace jail: only
// the audit's report step and the operator's recheck write it, so a run cannot edit a finding's history.
const lifeFileName = "finding-life.json"

const lifeLabel = "finding history"

func (m *WorkspaceManager) lifePath(workspaceID string) string {
	return filepath.Join(m.resolver.InternalDir(workspaceID), lifeFileName)
}

// ReadLife returns the workspace's finding life records. A missing file is none, not an error; an undecodable one is
// quarantined as *.corrupt and reported as an error. The file goes with the workspace when it is deleted.
func (m *WorkspaceManager) ReadLife(workspaceID string) ([]models.FindingLife, error) {
	return readJSONFile[[]models.FindingLife](m.lifePath(workspaceID), lifeLabel)
}

// WriteLife replaces the workspace's finding life records atomically.
func (m *WorkspaceManager) WriteLife(workspaceID string, records []models.FindingLife) error {
	if err := writeJSONFile(m.lifePath(workspaceID), "life-*.json.tmp", lifeLabel, records); err != nil {
		return fmt.Errorf("failed to save the finding history: %w", err)
	}
	return nil
}

// lifeMu serialises UpdateLife: the nightly report step and the operator's recheck both change the file, and an update that read
// the old records must not overwrite one that was written in between.
var lifeMu sync.Mutex

// UpdateLife reads the workspace's records, lets change replace them and writes the result, all under one lock.
func (m *WorkspaceManager) UpdateLife(workspaceID string, change func([]models.FindingLife) []models.FindingLife) error {
	lifeMu.Lock()
	defer lifeMu.Unlock()
	records, err := m.ReadLife(workspaceID)
	if err != nil {
		return err
	}
	return m.WriteLife(workspaceID, change(records))
}
