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
	// saveRetryDelay holds a reminder back when its state could not be saved, so a
	// store that is down cannot turn the loop into a spin that re-sends it.
	saveRetryDelay = 30 * time.Second
	deliverTimeout = 30 * time.Second
	latePrefix     = "(late) "
)

// Deliverer sends reminder text to a connector. connector is a connector type
// (for example "telegram"); Connectors lists the ones that are configured and enabled.
type Deliverer interface {
	Deliver(ctx context.Context, connector, message string) error
	Connectors() []string
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
	// Skipped returns the connector types ticked off for reminders (ChannelPolicy);
	// nil means none, so every enabled connector gets them.
	Skipped func() []string
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
	// lastEventAt is the time the newest outcome was saved under (see nextEventTime).
	lastEventAt time.Time
}

// NewService builds the service; call Run once from the app bootstrap layer.
func NewService(s *Store, d Deliverer, n Notice, c Clock, cfg Config) *Service {
	if cfg.Skipped == nil {
		cfg.Skipped = func() []string { return nil }
	}
	if cfg.Policy.MaxAttempts == 0 {
		cfg.Policy = DefaultPolicy()
	}
	return &Service{store: s, deliver: d, notice: n, clock: c, cfg: cfg,
		channel: ChannelPolicy{Connectors: d.Connectors, Skipped: cfg.Skipped}, wake: make(chan struct{}, 1), held: make(map[int64]time.Time)}
}

// Channels are the connectors a reminder with this stored channel choice will be
// delivered to; none means the web UI only.
func (s *Service) Channels(stored string) []string { return s.channel.Resolve(stored) }

// Channel names where a reminder with this stored choice goes, for people.
func (s *Service) Channel(stored string) string { return ChannelLabel(s.Channels(stored)) }

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
	// Facts about the schedule only: the message is the user's own text and is never logged.
	logging.Info("reminder created", "id", id, "fire_at", r.FireAt.UTC().Format(time.RFC3339), "cron", r.Cron,
		"zone", r.Timezone, "channel", r.Connector, "created_by", r.CreatedBy)
	return s.store.Get(ctx, id)
}

// Plan resolves and validates a described reminder exactly as Create would, without
// storing it: the reminder as it would be saved, or the reason it would be refused.
func (s *Service) Plan(r Reminder) (Reminder, error) { return s.prepare(r) }

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

// EventsAfter returns the reminders whose last outcome came after the given time, newest first and at most limit.
func (s *Service) EventsAfter(ctx context.Context, after time.Time, limit int) ([]Reminder, error) {
	return s.store.EventsAfter(ctx, after, limit)
}

// LatestEventAt is the time of the newest outcome saved on any reminder, zero when there is none.
func (s *Service) LatestEventAt(ctx context.Context) (time.Time, error) {
	return s.store.LatestEventAt(ctx)
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
		outcome = s.send(ctx, r, timing)
	}
	step := s.cfg.Policy.Advance(r, outcome, now)
	if step.Event != "" {
		step.Next.Event, step.Next.EventAt = step.Event, s.nextEventTime()
		s.notice.Notify(ctx, step.Next, step.Event)
	}
	s.save(ctx, step.Next, now)
}

// send delivers to every connector the reminder resolves to. It succeeds when at
// least one took it (the others' failures are reported, not retried: a retry would
// send the text again to the ones that already have it) and fails only when none did.
func (s *Service) send(ctx context.Context, r Reminder, timing Timing) Outcome {
	channels := s.Channels(r.Connector)
	msg := r.Message
	if timing == Late {
		msg = latePrefix + msg
	}
	var failures []string
	for _, channel := range channels {
		if err := s.deliverTo(ctx, channel, msg); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", channel, err))
		}
	}
	switch {
	case len(failures) == 0:
		return Delivered(timing)
	case len(failures) < len(channels):
		return DeliveredPartly(timing, strings.Join(failures, "; "))
	default:
		return Failed(strings.Join(failures, "; "))
	}
}

func (s *Service) deliverTo(ctx context.Context, channel, msg string) error {
	sendCtx, cancel := context.WithTimeout(ctx, deliverTimeout)
	defer cancel()
	return s.deliver.Deliver(sendCtx, channel, msg)
}

// save writes the new state. A cancel that won the race (ErrNotFound) is not an
// error; any other failure holds the reminder back instead of retrying at once.
// nextEventTime is the time to save an outcome under: the clock now, but always after the previous outcome's, because
// a client asks for what came after the newest it has seen and an event at an equal or older time would be skipped.
// Only the delivery loop calls it, so it needs no lock.
func (s *Service) nextEventTime() time.Time {
	at := s.clock.Now()
	if !at.After(s.lastEventAt) {
		at = s.lastEventAt.Add(time.Millisecond)
	}
	s.lastEventAt = at
	return at
}

func (s *Service) save(ctx context.Context, r Reminder, now time.Time) {
	err := s.store.Update(ctx, r)
	if err == nil || errors.Is(err, ErrNotFound) || ctx.Err() != nil {
		return
	}
	logging.Warn("reminders: update failed", "id", r.ID, "error", err)
	s.hold(r.ID, now)
}

func (s *Service) hold(id int64, now time.Time) { s.held[id] = now.Add(saveRetryDelay) }
