package netaudit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"llm-proxy/models"
)

const (
	// checkInEvery is the gap between two check-ins; checkInSlack lets the next one go at the same tick of the clock rather
	// than an hour later each day.
	checkInEvery = 24 * time.Hour
	checkInSlack = time.Hour
	// checkInTick is how often Run asks whether a check-in is due.
	checkInTick = 15 * time.Minute
)

// CheckInHistory keeps when the last check-in was sent: *History.
type CheckInHistory interface {
	LastCheckIn() (time.Time, error)
	MarkCheckIn(at time.Time) error
}

// CheckInDeps are what the daily check-in needs; the application wires them in one place.
type CheckInDeps struct {
	Enabled func() bool
	History CheckInHistory
	// Stalled lists the monitors that have stopped (MonitorWatch.Stalled).
	Stalled func() []MonitorStall
	Notify  func(ctx context.Context, message string) error
	Now     func() time.Time
	OnError func(error)
}

// CheckIn sends one short note a day while it is switched on, so a computer that went down is noticed by the note that did not
// come ("silence is never absence", applied to the program itself). It reads only what the monitor watch already holds.
type CheckIn struct {
	deps CheckInDeps
	last string // the trouble reported last, so a failure that repeats every tick is reported once
}

func NewCheckIn(deps CheckInDeps) *CheckIn {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &CheckIn{deps: deps}
}

// Run ticks until ctx ends; it is started from the one place the Network loops start.
func (c *CheckIn) Run(ctx context.Context) {
	ticker := time.NewTicker(checkInTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Tick(ctx)
		}
	}
}

// Tick sends the check-in when it is on and a day has passed. A note that could not be sent is not marked, so it goes at the
// next tick.
func (c *CheckIn) Tick(ctx context.Context) {
	if !c.deps.Enabled() {
		return
	}
	now := c.deps.Now()
	last, err := c.deps.History.LastCheckIn()
	if err != nil {
		c.report(err)
		return
	}
	if !last.IsZero() && now.Sub(last) < checkInEvery-checkInSlack {
		return
	}
	if err := c.deps.Notify(ctx, CheckInMessage(c.deps.Stalled())); err != nil {
		c.report(fmt.Errorf("daily check-in: could not send: %w", err))
		return
	}
	c.last = ""
	if err := c.deps.History.MarkCheckIn(now); err != nil {
		c.report(err)
	}
}

func (c *CheckIn) report(err error) {
	if err.Error() == c.last {
		return
	}
	c.last = err.Error()
	if c.deps.OnError != nil {
		c.deps.OnError(err)
	}
}

// CheckInMessage is the note: fixed words and the fixed names of the stopped monitors.
func CheckInMessage(stalls []MonitorStall) string {
	if len(stalls) == 0 {
		return "Daily check-in: this program is running, and every monitor that is switched on is working. If this note stops coming, check the computer."
	}
	words := make([]string, 0, len(stalls))
	for _, s := range stalls {
		words = append(words, s.Words)
	}
	return "Daily check-in: this program is running, but these monitors have stopped: " + strings.Join(words, "; ") + ". Open the Network page to see why."
}

// CheckInConfig reads and changes whether the daily check-in is on.
type CheckInConfig struct{ store PolicyStore }

func NewCheckInConfig(store PolicyStore) *CheckInConfig { return &CheckInConfig{store: store} }

// Enabled reports whether the daily check-in is switched on.
func (c *CheckInConfig) Enabled() bool { return c.store.Get().DailyCheckIn }

// Set switches the daily check-in on or off.
func (c *CheckInConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.DailyCheckIn = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the daily check-in setting: %w", err)
	}
	return nil
}
