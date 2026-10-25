package automation

import (
	"context"
	"errors"
	"sync"
)

// Preflight checks, before a run touches the model, that the run can do its job. A non-nil error ends the run as
// failed with that reason and wakes no model. Return a *PreflightError when the reason is something an operator can fix
// and should be told about once.
type Preflight func(ctx context.Context, entry *AutomationEntry) error

// WithPreflight installs a pre-flight check; nil (the default) runs every automation.
func WithPreflight(p Preflight) Option {
	return func(d *Dispatcher) { d.preflight = p }
}

// PreflightError is a pre-flight refusal with a stable Code, so the failure can be told to the operator once per
// reason, not every night.
type PreflightError struct {
	Code    string
	Message string
}

func (e *PreflightError) Error() string { return e.Message }

// reasonNoticeLimiter lets one notice per (automation, reason) through until the automation next succeeds. A nightly
// run that is blocked for the same reason every night tells the operator once, not every night. The zero value is
// ready to use.
type reasonNoticeLimiter struct {
	mu   sync.Mutex
	sent map[string]bool
}

func (l *reasonNoticeLimiter) reserve(automationKey, reason string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := automationKey + "\x00" + reason
	if l.sent[key] {
		return false
	}
	if l.sent == nil {
		l.sent = map[string]bool{}
	}
	l.sent[key] = true
	return true
}

func (l *reasonNoticeLimiter) release(automationKey, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.sent, automationKey+"\x00"+reason)
}

// reset forgets every reason noticed for the automation, so a later failure is told again.
func (l *reasonNoticeLimiter) reset(automationKey string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key := range l.sent {
		if len(key) > len(automationKey) && key[:len(automationKey)+1] == automationKey+"\x00" {
			delete(l.sent, key)
		}
	}
}

// notifyPreflight tells the operator once per reason that the run did not start.
func (d *Dispatcher) notifyPreflight(ctx context.Context, entry *AutomationEntry, err error) {
	var refusal *PreflightError
	if !d.wantsDelivery(entry) || !errors.As(err, &refusal) {
		return
	}
	key := failureNoticeKey(entry)
	if !d.preflightNotices.reserve(key, refusal.Code) {
		return
	}
	if d.send(ctx, entry, "⚠️ "+entry.Name+" did not run: "+refusal.Message) != nil {
		d.preflightNotices.release(key, refusal.Code)
	}
}
