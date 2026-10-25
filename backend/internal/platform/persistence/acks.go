package persistence

import (
	"fmt"
	"path/filepath"

	"llm-proxy/models"
)

// acksFileName holds a workspace's accepted findings, in the workspace's metadata folder outside the agent's workspace
// jail: only the operator's endpoint writes it, so a run cannot accept its own findings.
const acksFileName = "acks.json"

const acksLabel = "accepted findings"

func (m *WorkspaceManager) acksPath(workspaceID string) string {
	return filepath.Join(m.resolver.InternalDir(workspaceID), acksFileName)
}

// ReadAcks returns the workspace's accepted findings. A missing file is none, not an error; an undecodable one is
// quarantined as *.corrupt and reported as an error. The file goes with the workspace when it is deleted.
func (m *WorkspaceManager) ReadAcks(workspaceID string) ([]models.ScanAck, error) {
	return readJSONFile[[]models.ScanAck](m.acksPath(workspaceID), acksLabel)
}

// WriteAcks replaces the workspace's accepted findings atomically.
func (m *WorkspaceManager) WriteAcks(workspaceID string, acks []models.ScanAck) error {
	if err := writeJSONFile(m.acksPath(workspaceID), "acks-*.json.tmp", acksLabel, acks); err != nil {
		return fmt.Errorf("failed to save the accepted findings: %w", err)
	}
	return nil
}
