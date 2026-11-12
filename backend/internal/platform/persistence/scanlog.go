package persistence

import (
	"fmt"
	"path/filepath"
	"sync"

	"llm-proxy/models"
)

// scanLogFileName holds a workspace's scan log, beside the finding history and outside the agent's workspace jail: only the
// audit's report step writes it, so a run cannot edit what the network looked like.
const scanLogFileName = "scan-log.json"

const scanLogLabel = "scan log"

func (m *WorkspaceManager) scanLogPath(workspaceID string) string {
	return filepath.Join(m.resolver.InternalDir(workspaceID), scanLogFileName)
}

// ReadScanLog returns the workspace's scan log. A missing file is none, not an error; an undecodable one is quarantined as
// *.corrupt and reported as an error. The file goes with the workspace when it is deleted.
func (m *WorkspaceManager) ReadScanLog(workspaceID string) ([]models.ScanEntry, error) {
	return readJSONFile[[]models.ScanEntry](m.scanLogPath(workspaceID), scanLogLabel)
}

// WriteScanLog replaces the workspace's scan log atomically.
func (m *WorkspaceManager) WriteScanLog(workspaceID string, rows []models.ScanEntry) error {
	if err := writeJSONFile(m.scanLogPath(workspaceID), "scanlog-*.json.tmp", scanLogLabel, rows); err != nil {
		return fmt.Errorf("failed to save the scan log: %w", err)
	}
	return nil
}

// scanLogMu serialises UpdateScanLog: two audits of one workspace may finish together.
var scanLogMu sync.Mutex

// UpdateScanLog reads the workspace's rows, lets change replace them and writes the result, all under one lock.
func (m *WorkspaceManager) UpdateScanLog(workspaceID string, change func([]models.ScanEntry) []models.ScanEntry) error {
	scanLogMu.Lock()
	defer scanLogMu.Unlock()
	rows, err := m.ReadScanLog(workspaceID)
	if err != nil {
		return err
	}
	return m.WriteScanLog(workspaceID, change(rows))
}
