package reminders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"llm-proxy/internal/platform/logging"
)

const (
	// MaxOpen caps scheduled reminders so a runaway agent cannot fill the table.
	MaxOpen = 50
	// saveRetryDelay holds a reminder back when its state could not be saved, so a
	// store that is down cannot turn the loop into a spin that re-sends it.
	saveRetryDelay = 30 * time.Second
	deliverTimeout = 30 * time.Second
	latePrefix     = "(late) "
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
	// DefaultConnector returns the default channel setting: a connector type, or ""
	// for in-app only. A type that is not configured is ignored (ChannelPolicy).
	DefaultConnector func() string
	// Policy is the lifecycle numbers; the zero value means DefaultPolicy.
	Policy Policy
}

// Service owns the reminder lifecycle: creation, cancellation and the single
// timer loop that delivers them.
type Service struct {
	store   *Store
	deliver Deliverer
	notice  Notice
	clock   Clock
	cfg     Config
	channel ChannelPolicy
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
	if cfg.Policy.MaxAttempts == 0 {
		cfg.Policy = DefaultPolicy()
	}
	return &Service{store: s, deliver: d, notice: n, clock: c, cfg: cfg,
		channel: ChannelPolicy{Default: cfg.DefaultConnector, Configured: d.HasConnector}, wake: make(chan struct{}, 1), held: make(map[int64]time.Time)}
}

// Channel is where a reminder with this stored channel choice will be delivered.
func (s *Service) Channel(stored string) string { return s.channel.Resolve(stored) }

// Create validates and stores a reminder, then wakes the loop. A recurring
// reminder without FireAt starts at its next cron occurrence.
func (s *Service) Create(ctx context.Context, r Reminder) (Reminder, error) {
	r, err := s.prepare(r)
	if err != nil {
		return Reminder{}, err
	}
	if n, err := s.store.CountOpen(ctx); err != nil {
		return Reminder{}, err
	} else if n >= MaxOpen {
		return Reminder{}, fmt.Errorf("%w: too many open reminders (limit %d): cancel some first", ErrInvalid, MaxOpen)
	}
	id, err := s.store.Add(ctx, r)
	if err != nil {
		return Reminder{}, err
	}
	s.poke()
	return s.store.Get(ctx, id)
}

// Edit replaces the message, time, recurrence, end and channel of a reminder under
// the same rules as Create (its timezone is kept). A finished reminder (sent,
// missed, failed) is rescheduled by editing it; a paused one stays paused.
// ErrNotFound when it is cancelled or unknown; ErrInvalid (wrapped) when the new
// description is refused, in which case nothing changes.
func (s *Service) Edit(ctx context.Context, id int64, in Reminder) (Reminder, error) {
	cur, err := s.store.Get(ctx, id)
	if err != nil {
		return Reminder{}, err
	}
	in.ID, in.Timezone, in.CreatedBy = id, cur.Timezone, cur.CreatedBy
	in, err = s.prepare(in)
	if err != nil {
		return Reminder{}, err
	}
	if err := s.store.Replace(ctx, in); err != nil {
		return Reminder{}, err
	}
	s.poke()
	return s.store.Get(ctx, id)
}

// prepare resolves and validates a described reminder: a recurring one starts at
// its next occurrence, the time must be sensible, and a named channel must exist
// (in-app is always available).
func (s *Service) prepare(r Reminder) (Reminder, error) {
	now := s.clock.Now()
	if r.Cron != "" && r.FireAt.IsZero() {
		next, ok := NextFire(r, now)
		if !ok {
			return Reminder{}, fmt.Errorf("%w: recurrence never fires again", ErrInvalid)
		}
		r.FireAt = next
	}
	if err := ValidateNew(r, now); err != nil {
		return Reminder{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	if err := s.channel.ValidateNamed(r.Connector); err != nil {
		return Reminder{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	r.Connector = s.channel.ForNew(r.Connector, r.Origin)
	return r, nil
}

// Pause silences a repeating reminder without deleting it.
func (s *Service) Pause(ctx context.Context, id int64) (Reminder, error) {
	return s.change(ctx, id, StatusScheduled, func(r Reminder) (Reminder, error) { return Pause(r) })
}

// Resume brings a paused reminder back at its next occurrence.
func (s *Service) Resume(ctx context.Context, id int64) (Reminder, error) {
	return s.change(ctx, id, StatusPaused, func(r Reminder) (Reminder, error) { return Resume(r, s.clock.Now()) })
}

// Skip moves a repeating reminder past its next occurrence.
func (s *Service) Skip(ctx context.Context, id int64) (Reminder, error) {
	return s.change(ctx, id, StatusScheduled, func(r Reminder) (Reminder, error) { return SkipNext(r) })
}

// change reads a reminder, applies one of the pure state changes and writes the
// result only if it is still in the state it was read in, then re-arms the loop.
func (s *Service) change(ctx context.Context, id int64, from Status, apply func(Reminder) (Reminder, error)) (Reminder, error) {
	cur, err := s.store.Get(ctx, id)
	if err != nil {
		return Reminder{}, err
	}
	next, err := apply(cur)
	if err != nil {
		return Reminder{}, err
	}
	if err := s.store.Transition(ctx, next, from); err != nil {
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

// Remove deletes a reminder in any status (cancelling it first if it is open).
func (s *Service) Remove(ctx context.Context, id int64) error {
	err := s.store.Delete(ctx, id)
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
	open, err := s.store.ListScheduled(ctx)
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
	timing := s.cfg.Policy.Classify(r, now)
	outcome := Overdue
	if timing != Missed {
		outcome = Delivered(timing)
		if err := s.send(ctx, r, timing == Late); err != nil {
			outcome = Failed(err.Error())
		}
	}
	step := s.cfg.Policy.Advance(r, outcome, now)
	if step.Event != "" {
		s.notice.Notify(ctx, step.Next, step.Event)
	}
	s.save(ctx, step.Next, now)
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
