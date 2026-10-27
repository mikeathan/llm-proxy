package automation

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"time"

	"llm-proxy/internal/core/runlane"
	"llm-proxy/models"
)

// flockRetryInterval is the polling interval while waiting for the workspace
// flock (see acquireWorkspaceLock).
const flockRetryInterval = 250 * time.Millisecond

// RunOptions are what a manual trigger may change about one run. The zero value is the automation as saved.
type RunOptions struct {
	// RecordingRef plays back a recording instead of calling the model.
	RecordingRef string
	// ScanTarget points a network audit at one device for this run only, keeping the audit's own depth, ports and
	// checks. It is one IP address; the saved settings are never changed.
	ScanTarget string
}

// Errors a trigger returns for options it refuses.
var (
	ErrBadScanTarget = errors.New("a rescan takes one IP address")
	ErrNotAnAudit    = errors.New("only a network audit can scan a device")
)

func (o RunOptions) validateFor(entry *AutomationEntry) error {
	if o.ScanTarget == "" {
		return nil
	}
	if entry.Scan == nil {
		return ErrNotAnAudit
	}
	if _, err := netip.ParseAddr(o.ScanTarget); err != nil {
		return ErrBadScanTarget
	}
	return nil
}

// applyTo returns the entry this run executes: the entry itself, or a copy whose scan settings aim at ScanTarget.
func (o RunOptions) applyTo(entry *AutomationEntry) *AutomationEntry {
	if o.ScanTarget == "" || entry.Scan == nil {
		return entry
	}
	scan := *entry.Scan
	scan.Targets = []string{o.ScanTarget}
	run := *entry
	run.Scan = &scan
	return &run
}

// TriggerResult reports whether a manual trigger started immediately or was
// queued behind running work (with the 1-based queue position).
type TriggerResult struct {
	Status   string `json:"status"`
	Position int    `json:"position,omitempty"`
}

// Trigger admission statuses returned in TriggerResult.Status.
const (
	TriggerStarted string = "started"
	TriggerQueued  string = "queued"
	// TriggerSkipped: a skip_if_busy fire found the lane busy and was dropped.
	TriggerSkipped string = "skipped"
)

func (d *Dispatcher) Trigger(workspaceID, automationName string, opts RunOptions) (TriggerResult, error) {
	entry, ok := d.registry.Get(workspaceID, automationName)
	if !ok {
		return TriggerResult{}, fmt.Errorf("automation not found: %s/%s", workspaceID, automationName)
	}
	if err := opts.validateFor(entry); err != nil {
		return TriggerResult{}, err
	}
	return d.admitRun(entry, true, opts)
}

// admitRun submits an automation run to its workload-class lane and classifies
// the outcome. The lane queue dedupes by key, so a fire while the automation is
// running becomes its single pending rerun, and a fire while it is already
// queued is absorbed (ErrAlreadyQueued) and reported as queued at its existing
// position — never an error. The entry is re-resolved at dequeue so a deleted or
// renamed automation is dropped instead of running a stale definition.
func (d *Dispatcher) admitRun(entry *AutomationEntry, manual bool, opts RunOptions) (TriggerResult, error) {
	if d.skipHeartbeatTick(entry, manual) {
		return TriggerResult{Status: TriggerSkipped}, nil
	}
	sub, err := d.lane.Submit(runlane.Job{
		Key:         key(entry.Workspace, entry.Name),
		LaneKey:     d.laneKeyFor(entry.Model),
		WorkspaceID: entry.Workspace,
		Automation:  entry.Name,
		Label:       entry.Workspace + "/" + entry.Name,
		Kind:        runlane.KindAutomation,
		Manual:      manual,
		SkipIfBusy:  d.skipIfBusy(entry) && !manual,
		Model:       entry.Model,
		Run: func(ctx context.Context) error {
			live, ok := d.registry.Get(entry.Workspace, entry.Name)
			if !ok {
				return nil // automation deleted/renamed while queued — drop
			}
			// A queued tick may leave the queue after its window closed (or its
			// checks were removed): the same rule applies again at dequeue.
			if d.skipHeartbeatTick(live, manual) {
				return nil
			}
			return d.executeAutomation(ctx, opts.applyTo(live), opts.RecordingRef)
		},
	})
	switch {
	case errors.Is(err, runlane.ErrAlreadyQueued):
		return TriggerResult{Status: TriggerQueued, Position: sub.Position}, nil
	case err != nil:
		return TriggerResult{}, err
	case sub.Disposition == runlane.DispositionSkipped:
		d.metrics.RecordExecution(false, true, 0)
		d.recordHeartbeat(entry, models.HeartbeatSkippedBusy)
		d.logger.Info("automation tick skipped: run lane busy",
			"workspace", entry.Workspace, "automation", entry.Name)
		return TriggerResult{Status: TriggerSkipped}, nil
	case sub.Disposition == runlane.DispositionQueued:
		d.metrics.RecordQueued()
		return TriggerResult{Status: TriggerQueued, Position: sub.Position}, nil
	default:
		return TriggerResult{Status: TriggerStarted}, nil
	}
}

// acquireWorkspaceLock waits for the workspace flock instead of dropping the
// run: once the scheduler admits concurrent cloud runs, two automations in one
// workspace must queue on the lock, not discard each other. The wait is
// bounded by ctx — the run timeout (which starts at dequeue) is its ceiling.
func (d *Dispatcher) acquireWorkspaceLock(ctx context.Context, workspaceID string) (*os.File, error) {
	ticker := time.NewTicker(flockRetryInterval)
	defer ticker.Stop()
	for {
		f, err := d.persistence.TryAcquireLock(workspaceID)
		if err == nil {
			return f, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// ErrNoQueuedRun is returned by CancelQueued when the automation has no
// queued entry (it may still be running).
var ErrNoQueuedRun = errors.New("automation is not queued")

// CancelQueued drops a queued (not yet running) automation entry. A running
// automation is untouched — StopAutomation owns that path.
func (d *Dispatcher) CancelQueued(workspaceID, automationName string) error {
	if d.lane.CancelQueued(key(workspaceID, automationName)) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrNoQueuedRun, key(workspaceID, automationName))
}

// LaneSnapshot exposes the run scheduler read model for the API/UI.
func (d *Dispatcher) LaneSnapshot() runlane.Snapshot { return d.lane.Snapshot() }

func (d *Dispatcher) cancelQueuedForWorkspace(workspaceID string) {
	for _, ls := range d.lane.Snapshot().Lanes {
		for _, e := range ls.Queued {
			if e.WorkspaceID == workspaceID {
				d.lane.CancelQueued(e.Key)
			}
		}
	}
}
