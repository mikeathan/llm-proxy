package automation

import (
	"context"
	"errors"
	"time"

	"llm-proxy/internal/core/assistant/failures"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/models"
)

const (
	// deliveryTimeout bounds one connector send. It is a fresh child of the
	// dispatcher's context, not of the run's: a run that used its whole timeout
	// must still be able to report that it finished.
	deliveryTimeout = 30 * time.Second
	// seenPromptLimit caps how many already-reported titles are put in the prompt.
	seenPromptLimit = 25
)

// Notifier delivers a message through a named communication connector. It is
// an operator-configured, system-side send (like replying to an inbound
// message), not agent egress: the agent never calls it and cannot choose the
// destination.
type Notifier interface {
	Notify(ctx context.Context, connector, message string) error
}

// WithNotifier enables Notify delivery for automations that configure it.
func WithNotifier(n Notifier) Option {
	return func(d *Dispatcher) { d.notifier = n }
}

func (d *Dispatcher) wantsDelivery(entry *AutomationEntry) bool {
	return d.notifier != nil && entry.Notify != nil && entry.Notify.Connector != ""
}

// seenLedger loads the automation's seen ledger when dedup is on. An unreadable
// ledger degrades to "nothing seen yet" (and is rewritten after the next
// delivery) rather than failing the run.
func (d *Dispatcher) seenLedger(entry *AutomationEntry) models.SeenLedger {
	if !d.wantsDelivery(entry) || !entry.Notify.Dedup {
		return nil
	}
	ledger, err := d.persistence.ReadSeen(entry.Workspace, entry.Name)
	if err != nil {
		d.logger.Warn("seen ledger unreadable; treating as empty",
			"workspace", entry.Workspace, "automation", entry.Name, "error", err.Error())
		return nil
	}
	return ledger
}

// withSeenHint appends the already-reported titles to the task content so the
// run does not spend its search budget rediscovering them.
func withSeenHint(entry *AutomationEntry, task string, ledger models.SeenLedger) string {
	if len(ledger) == 0 {
		return task
	}
	titles := recentTitles(ledger, time.Now(), entry.Notify.RetentionDays(), seenPromptLimit)
	return task + prompts.AutomationSeenBlock(titles)
}

// deliverReport sends a finished run's report and, only once the send has
// succeeded, remembers the newly reported items — a failed delivery therefore
// re-offers the same items on the next run instead of losing them.
func (d *Dispatcher) deliverReport(ctx context.Context, entry *AutomationEntry, resp *ExecuteResponse, ledger models.SeenLedger) {
	if !d.wantsDelivery(entry) || resp == nil || resp.Report == "" {
		return
	}
	// A quiet heartbeat ("HEARTBEAT_OK") is the smart-skip signal: nothing to say.
	if isHeartbeatOK(resp.Report) {
		return
	}
	now := time.Now()
	dg := buildDigest(resp.Report, ledger, *entry.Notify, now)
	if dg.Message == "" {
		d.logger.Info("automation digest empty; nothing delivered",
			"workspace", entry.Workspace, "automation", entry.Name)
		return
	}
	if err := d.send(ctx, entry, dg.Message); err != nil {
		return
	}
	if !entry.Notify.Dedup || len(dg.NewItems) == 0 {
		return
	}
	merged := mergeSeen(ledger, dg.NewItems, now, entry.Notify.RetentionDays())
	if err := d.persistence.WriteSeen(entry.Workspace, entry.Name, merged); err != nil {
		d.logger.Warn("could not save seen ledger; items may be reported again",
			"workspace", entry.Workspace, "automation", entry.Name, "error", err.Error())
	}
}

// notifyFailure tells the operator an unattended run failed. A user-requested
// stop is not a failure worth a message.
func (d *Dispatcher) notifyFailure(ctx context.Context, entry *AutomationEntry, runErr error) {
	if !d.wantsDelivery(entry) || runErr == nil || errors.Is(runErr, context.Canceled) {
		return
	}
	msg := "⚠️ Automation " + entry.Name + " failed: " + failures.ClassifyRunFailure(runErr).Error
	_ = d.send(ctx, entry, msg)
}

// send delivers one message and logs (never propagates) a failure: delivery is
// best-effort and must not change the run's outcome.
func (d *Dispatcher) send(ctx context.Context, entry *AutomationEntry, message string) error {
	sendCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()
	err := d.notifier.Notify(sendCtx, entry.Notify.Connector, message)
	if err != nil {
		d.logger.Warn("automation delivery failed",
			"workspace", entry.Workspace, "automation", entry.Name,
			"connector", entry.Notify.Connector, "error", err.Error())
	}
	return err
}
