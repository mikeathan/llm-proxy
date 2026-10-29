package persistence

import (
	"fmt"
	"path/filepath"

	"llm-proxy/models"
)

// deepChecksFileName holds a workspace's last deep check of each device, beside the expectations and outside the agent's
// workspace jail: only the operator's endpoint writes it, so a run cannot write its own deep result.
const deepChecksFileName = "deepchecks.json"

const deepChecksLabel = "deep checks"

func (m *WorkspaceManager) deepChecksPath(workspaceID string) string {
	return filepath.Join(m.resolver.InternalDir(workspaceID), deepChecksFileName)
}

// ReadDeepChecks returns the workspace's stored deep checks. A missing file is none, not an error; an undecodable one is
// quarantined as *.corrupt and reported as an error.
func (m *WorkspaceManager) ReadDeepChecks(workspaceID string) ([]models.DeepCheck, error) {
	return readJSONFile[[]models.DeepCheck](m.deepChecksPath(workspaceID), deepChecksLabel)
}

// WriteDeepChecks replaces the workspace's stored deep checks atomically.
func (m *WorkspaceManager) WriteDeepChecks(workspaceID string, items []models.DeepCheck) error {
	if err := writeJSONFile(m.deepChecksPath(workspaceID), "deepchecks-*.json.tmp", deepChecksLabel, items); err != nil {
		return fmt.Errorf("failed to save the deep checks: %w", err)
	}
	return nil
}
