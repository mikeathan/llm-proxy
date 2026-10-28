package persistence

import (
	"fmt"
	"path/filepath"

	"llm-proxy/models"
)

// expectationsFileName holds a workspace's per-device expectations, beside the accepted findings and outside the agent's
// workspace jail: only the operator's endpoint writes it, so a run cannot declare its own findings expected.
const expectationsFileName = "expectations.json"

const expectationsLabel = "device expectations"

func (m *WorkspaceManager) expectationsPath(workspaceID string) string {
	return filepath.Join(m.resolver.InternalDir(workspaceID), expectationsFileName)
}

// ReadExpectations returns the workspace's device expectations. A missing file is none, not an error; an undecodable one
// is quarantined as *.corrupt and reported as an error. The file goes with the workspace when it is deleted.
func (m *WorkspaceManager) ReadExpectations(workspaceID string) ([]models.Expectation, error) {
	return readJSONFile[[]models.Expectation](m.expectationsPath(workspaceID), expectationsLabel)
}

// WriteExpectations replaces the workspace's device expectations atomically.
func (m *WorkspaceManager) WriteExpectations(workspaceID string, items []models.Expectation) error {
	if err := writeJSONFile(m.expectationsPath(workspaceID), "expectations-*.json.tmp", expectationsLabel, items); err != nil {
		return fmt.Errorf("failed to save the device expectations: %w", err)
	}
	return nil
}
