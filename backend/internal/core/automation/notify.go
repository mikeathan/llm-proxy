package automation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"llm-proxy/internal/core/assistant/failures"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/models"
)

const (
	// deliveryTimeout bounds one connector send. The send derives from the
	// lane context handed to executeAutomation, not from the run's own
	// timeout context: a run that used its whole timeout can still report that
	// it finished, while lane preemption or shutdown cancels the send.
	deliveryTimeout = 30 * time.Second
	// seenPromptLimit caps how many already-reported titles are put in the prompt.
	seenPromptLimit = 25
	// failureNoticeCooldown is the minimum gap between failure notices for one
	// automation, so a frequent schedule against a dead model does not flood
	// the connector. A successful run re-arms the notice immediately.
	failureNoticeCooldown = time.Hour
)

// failureNoticeLimiter remembers when each automation last sent a failure
// notice. The zero value is ready to use; entries are removed on recovery, so
// it is bounded by the number of currently failing automations.
type failureNoticeLimiter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// reserve claims the right to send a notice for key: it reports whether one may
// be sent now and, when it may, records the send so a concurrent failure of the
// same automation cannot also pass. A send that then fails calls release.
func (l *failureNoticeLimiter) reserve(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if at, ok := l.last[key]; ok && now.Sub(at) < failureNoticeCooldown {
		return false
	}
	if l.last == nil {
		l.last = map[string]time.Time{}
	}
	l.last[key] = now
	return true
}

// release gives back a reservation whose send failed, so the next failure may try again.
func (l *failureNoticeLimiter) release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.last, key)
}

func (l *failureNoticeLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.last, key)
}

func failureNoticeKey(entry *AutomationEntry) string {
	return entry.Workspace + "/" + entry.Name
}

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

// Delivery warnings: recorded on the run when its report could not be sent, so a run never looks delivered when it
// was not. The cause is clipped; notifier errors carry the platform's reply, never the credential.
const (
	deliveryWarningFormat   = "report not delivered via %s: %s"
	deliveryWarningMaxCause = 300
)

// deliverAndRecord sends a finished run's report and records what the operator needs to know about the send: a
// failure becomes a warning on the run that produced the report, and a heartbeat's result says whether its alert
// actually went out. It runs while the workspace lock is still held.
func (d *Dispatcher) deliverAndRecord(ctx context.Context, entry *AutomationEntry, resp *ExecuteResponse, ledger models.SeenLedger) {
	if resp == nil {
		return
	}
	sendErr := d.deliverReport(ctx, entry, resp, ledger)
	d.recordHeartbeat(entry, heartbeatOutcome(resp, sendErr))
	if sendErr != nil {
		d.recordDeliveryWarning(entry, resp.RunID, sendErr)
	}
}

// heartbeatOutcome is a heartbeat check's result once delivery is known: an alert whose send failed is not "sent".
func heartbeatOutcome(resp *ExecuteResponse, sendErr error) models.HeartbeatResult {
	result := heartbeatResultOf(resp)
	if result == models.HeartbeatAlert && sendErr != nil {
		return models.HeartbeatAlertNotDelivered
	}
	return result
}

// recordDeliveryWarning adds the delivery failure to the run's stored record (history and latest run).
func (d *Dispatcher) recordDeliveryWarning(entry *AutomationEntry, runID string, sendErr error) {
	cause := models.UnavailableReason(sendErr)
	if len(cause) > deliveryWarningMaxCause {
		cause = cause[:deliveryWarningMaxCause] + "…"
	}
	warning := fmt.Sprintf(deliveryWarningFormat, entry.Notify.Connector, cause)
	state, err := d.persistence.ReadState(entry.Workspace)
	if err != nil {
		d.logger.Warn("could not record the delivery warning", "workspace", entry.Workspace, "automation", entry.Name, "error", err.Error())
		return
	}
	if !state.AddRunWarning(runID, warning) {
		d.logger.Warn("delivery warning: run not found in state", "workspace", entry.Workspace, "automation", entry.Name, "run", runID)
		return
	}
	if err := d.persistence.WriteState(entry.Workspace, state); err != nil {
		d.logger.Warn("could not save the delivery warning", "workspace", entry.Workspace, "automation", entry.Name, "error", err.Error())
	}
}

// deliverReport sends a finished run's report and, only once the send has
// succeeded, remembers the newly reported items — a failed delivery therefore
// re-offers the same items on the next run instead of losing them. It returns
// the send error; skipping (no delivery configured, a quiet heartbeat, nothing
// new) is not an error.
func (d *Dispatcher) deliverReport(ctx context.Context, entry *AutomationEntry, resp *ExecuteResponse, ledger models.SeenLedger) error {
	if !d.wantsDelivery(entry) || resp.Report == "" {
		return nil
	}
	if isQuietHeartbeat(resp.Report) {
		return nil
	}
	now := time.Now()
	dg := buildDigest(resp.Report, ledger, *entry.Notify, now)
	if dg.Message == "" {
		d.logger.Info("automation digest empty; nothing delivered",
			"workspace", entry.Workspace, "automation", entry.Name)
		return nil
	}
	if err := d.send(ctx, entry, dg.Message); err != nil {
		return err
	}
	if !entry.Notify.Dedup || len(dg.NewItems) == 0 {
		return nil
	}
	merged := mergeSeen(ledger, dg.NewItems, now, entry.Notify.RetentionDays())
	if err := d.persistence.WriteSeen(entry.Workspace, entry.Name, merged); err != nil {
		d.logger.Warn("could not save seen ledger; items may be reported again",
			"workspace", entry.Workspace, "automation", entry.Name, "error", err.Error())
	}
	return nil
}

// notifyFailure tells the operator an unattended run failed, at most once per
// failureNoticeCooldown per automation. A user-requested stop is not a failure
// worth a message.
func (d *Dispatcher) notifyFailure(ctx context.Context, entry *AutomationEntry, runErr error) {
	if !d.wantsDelivery(entry) || runErr == nil || errors.Is(runErr, context.Canceled) {
		return
	}
	key := failureNoticeKey(entry)
	if !d.failureNotices.reserve(key, time.Now()) {
		return
	}
	msg := "⚠️ Automation " + entry.Name + " failed: " + failures.ClassifyRunFailure(runErr).Error
	if d.send(ctx, entry, msg) != nil {
		d.failureNotices.release(key)
	}
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
