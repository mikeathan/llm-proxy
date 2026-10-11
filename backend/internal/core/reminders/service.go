package reminders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/platform/logging"
)

const (
	// MaxOpen caps scheduled reminders so a runaway agent cannot fill the table.
	MaxOpen = 50
	// lateWindow: a reminder found overdue by less than this is still delivered
	// (marked late); older ones become "missed" and are shown in-app only.
	lateWindow = 12 * time.Hour
	// lateGrace is the normal timer jitter below which a delivery is not "late".
	lateGrace = 2 * time.Minute
	// maxAttempts is the number of delivery tries before a reminder is failed.
	maxAttempts = 3
	// saveRetryDelay holds a reminder back when its state could not be saved, so a
	// store that is down cannot turn the loop into a spin that re-sends it.
	saveRetryDelay  = 30 * time.Second
	deliverTimeout  = 30 * time.Second
	InAppChannel    = "in-app"
	eventDue        = "due"
	eventLate       = "late"
	eventMissed     = "missed"
	eventFailed     = "failed"
	latePrefix      = "(late) "
	defaultFirstTry = time.Minute
)

// Deliverer sends reminder text to a connector. connector is a connector type
// (for example "telegram"); HasConnector reports whether one is configured.
type Deliverer interface {
	Deliver(ctx context.Context, connector, message string) error
	HasConnector(connector string) bool
}

// Notice is the in-app sink: every delivery outcome is recorded there so a
// reminder is never silently lost even when no connector is configured.
type Notice interface {
	Notify(ctx context.Context, r Reminder, event string)
}

// Timer is the part of time.Timer the scheduler needs.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// Clock lets tests drive time.
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

type realClock struct{}

type realTimer struct{ *time.Timer }

func (t realTimer) C() <-chan time.Time { return t.Timer.C }

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }

// SystemClock is the wall clock.
func SystemClock() Clock { return realClock{} }

// Config is read live so a settings change applies without a restart.
type Config struct {
	// DefaultConnector returns the connector type used when a reminder names none;
	// empty means in-app only.
	DefaultConnector func() string
	// RetryDelays are the waits before retry 2, 3, … (default 1m then 5m).
	RetryDelays []time.Duration
}

// Service owns the reminder lifecycle: creation, cancellation and the single
// timer loop that delivers them.
type Service struct {
	store   *Store
	deliver Deliverer
	notice  Notice
	clock   Clock
	cfg     Config
	wake    chan struct{}
	// held maps a reminder whose state could not be saved to the time before which
	// it is not retried. Touched only by the Run goroutine.
	held map[int64]time.Time
}

// NewService builds the service; call Run once from the app bootstrap layer.
func NewService(s *Store, d Deliverer, n Notice, c Clock, cfg Config) *Service {
	if cfg.DefaultConnector == nil {
		cfg.DefaultConnector = func() string { return "" }
	}
	if len(cfg.RetryDelays) == 0 {
		cfg.RetryDelays = []time.Duration{defaultFirstTry, 5 * time.Minute}
	}
	return &Service{store: s, deliver: d, notice: n, clock: c, cfg: cfg, wake: make(chan struct{}, 1), held: make(map[int64]time.Time)}
}

// HasConnector reports whether a connector of that type is configured.
func (s *Service) HasConnector(connector string) bool { return s.deliver.HasConnector(connector) }

// Channel is where a reminder naming `connector` (or none) will be delivered.
func (s *Service) Channel(connector string) string {
	if connector != "" {
		return connector
	}
	if def := s.cfg.DefaultConnector(); def != "" {
		return def
	}
	return InAppChannel
}

// Create validates and stores a reminder, then wakes the loop. A recurring
// reminder without FireAt starts at its next cron occurrence.
func (s *Service) Create(ctx context.Context, r Reminder) (Reminder, error) {
	now := s.clock.Now()
	if r.Cron != "" && r.FireAt.IsZero() {
		next, ok := NextFire(r, now)
		if !ok {
			return Reminder{}, errors.New("recurrence never fires again")
		}
		r.FireAt = next
	}
	if err := ValidateNew(r, now); err != nil {
		return Reminder{}, err
	}
	if r.Connector != "" && !s.deliver.HasConnector(r.Connector) {
		return Reminder{}, fmt.Errorf("no %q connector is configured: enable it in Settings or choose another channel", r.Connector)
	}
	if n, err := s.store.CountOpen(ctx); err != nil {
		return Reminder{}, err
	} else if n >= MaxOpen {
		return Reminder{}, fmt.Errorf("too many open reminders (limit %d): cancel some first", MaxOpen)
	}
	id, err := s.store.Add(ctx, r)
	if err != nil {
		return Reminder{}, err
	}
	s.poke()
	return s.store.Get(ctx, id)
}

// Cancel cancels an open reminder.
func (s *Service) Cancel(ctx context.Context, id int64) error {
	err := s.store.Cancel(ctx, id)
	if err == nil {
		s.poke()
	}
	return err
}

// List returns the open reminders, soonest first.
func (s *Service) List(ctx context.Context) ([]Reminder, error) { return s.store.ListOpen(ctx) }

// Recent returns finished reminders for the in-app notice list.
func (s *Service) Recent(ctx context.Context, limit int) ([]Reminder, error) {
	return s.store.Recent(ctx, limit)
}

func (s *Service) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run delivers reminders until ctx is cancelled. It sleeps until the earliest
// scheduled time (no polling tick) and re-arms whenever a reminder is created or
// cancelled. Overdue reminders found at start-up follow the late/missed policy.
func (s *Service) Run(ctx context.Context) {
	for {
		s.processDue(ctx)
		timer := s.armTimer(ctx)
		var fire <-chan time.Time
		if timer != nil {
			fire = timer.C()
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-s.wake:
		case <-fire:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

// armTimer returns a timer for the earliest open reminder, or nil when none. A
// reminder held back after a failed save is not due again before its hold ends.
func (s *Service) armTimer(ctx context.Context) Timer {
	open, err := s.store.ListOpen(ctx)
	if err != nil || len(open) == 0 {
		if err != nil && ctx.Err() == nil {
			logging.Warn("reminders: list failed", "error", err)
		}
		return nil
	}
	now := s.clock.Now()
	next := s.effectiveTime(open[0])
	for _, r := range open[1:] {
		if t := s.effectiveTime(r); t.Before(next) {
			next = t
		}
	}
	return s.clock.NewTimer(max(next.Sub(now), 0))
}

func (s *Service) effectiveTime(r Reminder) time.Time {
	if until, ok := s.held[r.ID]; ok && until.After(r.FireAt) {
		return until
	}
	return r.FireAt
}

func (s *Service) processDue(ctx context.Context) {
	due, err := s.store.Due(ctx, s.clock.Now())
	if err != nil {
		if ctx.Err() == nil {
			logging.Warn("reminders: due query failed", "error", err)
		}
		return
	}
	for _, r := range due {
		if ctx.Err() != nil {
			return
		}
		// Time is read per reminder: a slow send must not make the next one look on time.
		now := s.clock.Now()
		if until, ok := s.held[r.ID]; ok {
			if now.Before(until) {
				continue
			}
			delete(s.held, r.ID)
		}
		s.handle(ctx, r, now)
	}
}

func (s *Service) handle(ctx context.Context, r Reminder, now time.Time) {
	// A cancel may land between the Due query and here.
	if cur, err := s.store.Get(ctx, r.ID); err != nil || cur.Status != StatusScheduled {
		if err != nil && !errors.Is(err, ErrNotFound) {
			s.hold(r.ID, now)
		}
		return
	}
	overdue := now.Sub(r.FireAt)
	if overdue > lateWindow {
		s.markMissed(ctx, r, now)
		return
	}
	late := overdue > lateGrace
	err := s.send(ctx, r, late)
	if err != nil {
		s.fail(ctx, r, now, err)
		return
	}
	event := eventDue
	if late {
		event = eventLate
	}
	s.notice.Notify(ctx, r, event)
	s.finish(ctx, r, now)
}

func (s *Service) send(ctx context.Context, r Reminder, late bool) error {
	channel := s.Channel(r.Connector)
	if channel == InAppChannel {
		return nil
	}
	msg := r.Message
	if late {
		msg = latePrefix + msg
	}
	sendCtx, cancel := context.WithTimeout(ctx, deliverTimeout)
	defer cancel()
	return s.deliver.Deliver(sendCtx, channel, msg)
}

// finish closes a one-off or advances a recurring reminder to its next time.
func (s *Service) finish(ctx context.Context, r Reminder, now time.Time) {
	r.Attempts, r.LastError = 0, ""
	if next, ok := NextFire(r, now); r.Cron != "" && ok {
		r.FireAt = next
	} else {
		r.Status = StatusDone
	}
	s.save(ctx, r, now)
}

func (s *Service) markMissed(ctx context.Context, r Reminder, now time.Time) {
	s.notice.Notify(ctx, r, eventMissed)
	if next, ok := NextFire(r, now); r.Cron != "" && ok {
		r.FireAt, r.Attempts, r.LastError = next, 0, ""
	} else {
		r.Status = StatusMissed
	}
	s.save(ctx, r, now)
}

// fail records a failed send. After maxAttempts a one-off becomes failed; a
// recurring reminder reports the failure and moves on to its next occurrence, so
// a short connector outage does not end it for good.
func (s *Service) fail(ctx context.Context, r Reminder, now time.Time, cause error) {
	r.Attempts++
	r.LastError = truncate(cause.Error(), 300)
	switch {
	case r.Attempts < maxAttempts:
		r.FireAt = now.Add(s.retryDelay(r.Attempts))
	default:
		s.notice.Notify(ctx, r, eventFailed)
		if next, ok := NextFire(r, now); r.Cron != "" && ok {
			r.FireAt, r.Attempts = next, 0
		} else {
			r.Status = StatusFailed
		}
	}
	s.save(ctx, r, now)
}

func (s *Service) retryDelay(attempts int) time.Duration {
	i := attempts - 1
	if i >= len(s.cfg.RetryDelays) {
		i = len(s.cfg.RetryDelays) - 1
	}
	return s.cfg.RetryDelays[i]
}

// save writes the new state. A cancel that won the race (ErrNotFound) is not an
// error; any other failure holds the reminder back instead of retrying at once.
func (s *Service) save(ctx context.Context, r Reminder, now time.Time) {
	err := s.store.Update(ctx, r)
	if err == nil || errors.Is(err, ErrNotFound) || ctx.Err() != nil {
		return
	}
	logging.Warn("reminders: update failed", "id", r.ID, "error", err)
	s.hold(r.ID, now)
}

func (s *Service) hold(id int64, now time.Time) { s.held[id] = now.Add(saveRetryDelay) }

func truncate(msg string, n int) string {
	msg = strings.TrimSpace(msg)
	if len(msg) <= n {
		return msg
	}
	return msg[:n]
}
