package automation

import (
	"sort"
	"sync/atomic"
	"time"

	"llm-proxy/models"
)

const MaxHistorySize = 100

// GlobalActivity returns the rolling global ledger of recent events.
func (d *Dispatcher) GlobalActivity() []models.AutomationRun {
	d.historyMu.RLock()
	defer d.historyMu.RUnlock()

	// Return a copy to avoid data races
	res := make([]models.AutomationRun, len(d.globalHistory))
	copy(res, d.globalHistory)
	return res
}

func (d *Dispatcher) RecordActivity(run models.AutomationRun) {
	d.historyMu.Lock()
	defer d.historyMu.Unlock()

	d.globalHistory = append(d.globalHistory, run)
	if len(d.globalHistory) > MaxHistorySize {
		d.globalHistory = d.globalHistory[len(d.globalHistory)-MaxHistorySize:]
	}
}

// ClearWorkspaceHistory purges all runs for a specific workspace from the global history.
func (d *Dispatcher) ClearWorkspaceHistory(workspaceID string) {
	d.historyMu.Lock()
	defer d.historyMu.Unlock()

	var newHistory []models.AutomationRun
	for _, run := range d.globalHistory {
		if run.WorkspaceID != workspaceID {
			newHistory = append(newHistory, run)
		}
	}
	d.globalHistory = newHistory
}

// LoadHistory populates the global history from persistent workspace states.
func (d *Dispatcher) LoadHistory() {
	workspaces, err := d.persistence.ListWorkspaces()
	if err != nil {
		d.logger.Error("Failed to list workspaces for history load", "error", err)
		return
	}

	var allRuns []models.AutomationRun
	for _, ws := range workspaces {
		state, err := d.persistence.ReadState(ws.ID)
		if err == nil {
			// Ensure WorkspaceID is set even for legacy records
			for i := range state.History {
				if state.History[i].WorkspaceID == "" {
					state.History[i].WorkspaceID = ws.ID
				}
			}
			allRuns = append(allRuns, state.History...)
		}
	}

	// Sort chronologically (oldest to newest)
	sort.Slice(allRuns, func(i, j int) bool {
		return allRuns[i].Timestamp.Before(allRuns[j].Timestamp)
	})

	d.historyMu.Lock()
	defer d.historyMu.Unlock()

	if len(allRuns) > MaxHistorySize {
		allRuns = allRuns[len(allRuns)-MaxHistorySize:]
	}
	d.globalHistory = allRuns

	// Reset and recalculate metrics from history. The counters are stored
	// atomically to match RecordExecution (which adds to them atomically);
	// TotalLatency is mutex-guarded, matching RecordExecution. Mixing atomic
	// and plain access to the same field would be a data race.
	var total, successful, failed int64
	var totalLatency time.Duration
	for _, run := range allRuns {
		total++
		totalLatency += time.Duration(run.DurationMs) * time.Millisecond
		if run.Error == "" {
			successful++
		} else {
			failed++
		}
	}
	atomic.StoreInt64(&d.metrics.TotalExecutions, total)
	atomic.StoreInt64(&d.metrics.SuccessfulExecutions, successful)
	atomic.StoreInt64(&d.metrics.FailedExecutions, failed)
	atomic.StoreInt64(&d.metrics.SkippedExecutions, 0)
	d.metrics.mu.Lock()
	d.metrics.TotalLatency = totalLatency
	d.metrics.mu.Unlock()

	d.logger.Info("Loaded global history", "count", len(d.globalHistory), "total_executions", total)
}
