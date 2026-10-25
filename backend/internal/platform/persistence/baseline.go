package persistence

import (
	"fmt"

	"llm-proxy/models"
)

// baselineDirName is the metadata subfolder holding per-automation scan baselines, beside the seen ledgers and outside
// the agent's workspace jail, so a run cannot edit the result it will be compared with.
const baselineDirName = "baseline"

const baselineLabel = "scan baseline"

func (m *WorkspaceManager) baselinePath(workspaceID, automation string) string {
	return m.automationFilePath(workspaceID, baselineDirName, automation, ".json")
}

// ReadBaseline returns an automation's latest scan result. A missing file is a zero baseline (no earlier scan), not an
// error; an undecodable one is quarantined as *.corrupt and reported as an error.
func (m *WorkspaceManager) ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error) {
	return readJSONFile[models.ScanBaseline](m.baselinePath(workspaceID, automation), baselineLabel)
}

// WriteBaseline replaces an automation's baseline atomically.
func (m *WorkspaceManager) WriteBaseline(workspaceID, automation string, b models.ScanBaseline) error {
	if err := writeJSONFile(m.baselinePath(workspaceID, automation), "baseline-*.json.tmp", baselineLabel, b); err != nil {
		return fmt.Errorf("failed to save the scan baseline: %w", err)
	}
	return nil
}

// DeleteBaseline removes an automation's baseline and any quarantined copy. A missing file is not an error.
func (m *WorkspaceManager) DeleteBaseline(workspaceID, automation string) error {
	return removeJSONFile(m.baselinePath(workspaceID, automation), baselineLabel)
}
