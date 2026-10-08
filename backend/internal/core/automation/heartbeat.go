// heartbeat.go — the run-time rules of the workspace heartbeat (models.HeartbeatConfig): a tick with no
// checks never reaches the model, skip-if-busy follows the model's lane, and each check's outcome is recorded.
package automation

import (
	"fmt"
	"time"

	"llm-proxy/internal/core/runlane"
	"llm-proxy/models"
)

func isHeartbeat(entry *AutomationEntry) bool {
	return entry.Name == models.HeartbeatAutomationName
}

// heartbeatHasChecks reports whether heartbeat.md holds anything to check; an unreadable file counts as none.
func (d *Dispatcher) heartbeatHasChecks(workspaceID string) bool {
	content, err := d.persistence.ReadTaskFile(workspaceID, models.HeartbeatFilename)
	return err == nil && models.HeartbeatBody(content) != ""
}

// skipIfBusy decides whether a scheduled fire is dropped when its lane is busy. A heartbeat only contends for
// the GPU on the local lane, so only there is a tick disposable; it is resolved at fire time because an empty
// model follows the registry primary.
func (d *Dispatcher) skipIfBusy(entry *AutomationEntry) bool {
	if isHeartbeat(entry) {
		return d.laneKeyFor(entry.Model) == runlane.LaneLocal
	}
	return entry.SkipIfBusy
}

// recordHeartbeat stores how the latest heartbeat check ended; it is a no-op for any other automation.
func (d *Dispatcher) recordHeartbeat(entry *AutomationEntry, result models.HeartbeatResult) {
	if !isHeartbeat(entry) {
		return
	}
	status := models.HeartbeatStatus{At: time.Now(), Result: result}
	if err := d.persistence.WriteHeartbeatStatus(entry.Workspace, status); err != nil {
		d.logger.Warn("failed to record heartbeat status", "workspace", entry.Workspace, "error", err)
	}
}

// HeartbeatState is the Heartbeat panel's view of a workspace.
func (d *Dispatcher) HeartbeatState(workspaceID string) (models.HeartbeatState, error) {
	cfg, err := d.persistence.ReadConfig(workspaceID)
	if err != nil {
		return models.HeartbeatState{}, fmt.Errorf("failed to read workspace config: %w", err)
	}
	var state models.HeartbeatState
	if cfg.Heartbeat != nil {
		state.Config = *cfg.Heartbeat
	}
	lane := d.laneKeyFor(state.Config.Model)
	state.Lane = string(lane)
	state.WakesLocalModel = lane == runlane.LaneLocal
	state.HasChecks = d.heartbeatHasChecks(workspaceID)
	if status, err := d.persistence.ReadHeartbeatStatus(workspaceID); err != nil {
		d.logger.Warn("unreadable heartbeat status", "workspace", workspaceID, "error", err)
	} else {
		state.Status = status
	}
	return state, nil
}
