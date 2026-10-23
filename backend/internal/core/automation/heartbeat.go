// heartbeat.go — the run-time rules of the workspace heartbeat (models.HeartbeatConfig): a tick with no
// checks or outside the active hours never reaches the model, skip-if-busy follows the model's lane, each check's outcome is recorded,
// and a report that starts with HEARTBEAT_OK is the quiet "nothing to report" signal (Pulse Logic).
package automation

import (
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/core/runlane"
	"llm-proxy/models"
)

func isHeartbeat(entry *AutomationEntry) bool {
	return entry.Name == models.HeartbeatAutomationName
}

// heartbeatSkip says whether a heartbeat tick must be dropped before it reaches the lane, and why. A scheduled
// tick outside the configured active hours is dropped (a manual run ignores the window, like skip-if-busy), as is
// any tick with no checks to run. Other automations are never skipped here.
func (d *Dispatcher) heartbeatSkip(entry *AutomationEntry, manual bool) (models.HeartbeatResult, bool) {
	if !isHeartbeat(entry) {
		return "", false
	}
	if !manual && !d.heartbeatActive(entry.Workspace) {
		return models.HeartbeatSkippedOutsideHours, true
	}
	if !d.heartbeatHasChecks(entry.Workspace) {
		return models.HeartbeatSkippedNoChecks, true
	}
	return "", false
}

// skipHeartbeatTick applies heartbeatSkip and, when the tick is dropped, records why. It is checked when a tick is
// admitted and again when it leaves the queue, so a queued tick cannot run after its window closed.
func (d *Dispatcher) skipHeartbeatTick(entry *AutomationEntry, manual bool) bool {
	reason, skip := d.heartbeatSkip(entry, manual)
	if !skip {
		return false
	}
	d.metrics.RecordExecution(false, true, 0)
	d.recordHeartbeat(entry, reason)
	d.logger.Info("heartbeat tick skipped", "workspace", entry.Workspace, "reason", string(reason))
	return true
}

// heartbeatActive reads the workspace's current active hours at fire time, so a changed window applies to the
// very next tick without re-registering. An unreadable config keeps the heartbeat running.
func (d *Dispatcher) heartbeatActive(workspaceID string) bool {
	cfg, err := d.persistence.ReadConfig(workspaceID)
	if err != nil {
		d.logger.Warn("cannot read heartbeat active hours; treating as active", "workspace", workspaceID, "error", err)
		return true
	}
	return cfg.Heartbeat == nil || cfg.Heartbeat.ActiveAt(d.now())
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
	status := models.HeartbeatStatus{At: d.now(), Result: result}
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

// ============================================================================
// Pulse Logic (Smart Skip)
// ============================================================================

const (
	heartbeatOKMarker = "HEARTBEAT_OK"
	// heartbeatMarkupChars are markdown emphasis characters a model may wrap
	// around the marker (**HEARTBEAT_OK**, `HEARTBEAT_OK`).
	heartbeatMarkupChars = "*_`# \t\r\n"
)

// isQuietHeartbeat reports whether a report is the "nothing to report" signal:
// it must START with the marker (ignoring markdown emphasis), so a real alert
// that merely mentions the marker is never swallowed. The UI's smart skip and
// connector delivery share this one definition.
func isQuietHeartbeat(report string) bool {
	return strings.HasPrefix(strings.TrimLeft(report, heartbeatMarkupChars), heartbeatOKMarker)
}

// isQuietResponse judges the agent's report alone when there is one — Output
// carries a run header and would match a marker mentioned anywhere. A response
// without a Report (legacy executors) falls back to scanning Output.
func isQuietResponse(resp *ExecuteResponse) bool {
	if resp.Report != "" {
		return isQuietHeartbeat(resp.Report)
	}
	return strings.Contains(resp.Output, heartbeatOKMarker)
}

// heartbeatResultOf classifies a finished heartbeat check by its report.
func heartbeatResultOf(resp *ExecuteResponse) models.HeartbeatResult {
	if isQuietResponse(resp) {
		return models.HeartbeatQuiet
	}
	return models.HeartbeatAlert
}

// ApplyPulseLogic applies the Smart Skip logic to suppress noisy heartbeat output.
func ApplyPulseLogic(resp *ExecuteResponse) {
	if isQuietResponse(resp) {
		resp.Output = ""
		resp.State.LastPulse = time.Now()
	}
}
