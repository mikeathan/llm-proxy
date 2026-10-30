package netaudit

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"llm-proxy/models"
)

// AllowedPresenceIntervals are the minutes between presence checks an operator may choose.
var AllowedPresenceIntervals = []int{5, 10, 15, 30}

// DefaultPresenceInterval is the interval used until the operator chooses one.
const DefaultPresenceInterval = 10

// ErrBadPresenceInterval is returned for an interval that is not one of AllowedPresenceIntervals.
var ErrBadPresenceInterval = errors.New("that interval is not one of the choices")

// PresenceView is the presence check's settings as the operator sees them.
type PresenceView struct {
	Enabled          bool  `json:"enabled"`
	IntervalMinutes  int   `json:"interval_minutes"`
	AllowedIntervals []int `json:"allowed_intervals"`
}

// PresenceConfig reads and changes the presence check's settings in the network-audit policy.
type PresenceConfig struct{ store PolicyStore }

func NewPresenceConfig(store PolicyStore) *PresenceConfig { return &PresenceConfig{store: store} }

func interval(minutes int) int {
	if minutes == 0 {
		return DefaultPresenceInterval
	}
	return minutes
}

// View returns the stored settings with the default filled in.
func (c *PresenceConfig) View() PresenceView {
	cfg := c.store.Get()
	return PresenceView{Enabled: cfg.PresenceCheck, IntervalMinutes: interval(cfg.PresenceIntervalMinutes), AllowedIntervals: AllowedPresenceIntervals}
}

// Set turns the check on or off and chooses how often it runs; 0 minutes is the default. A refused interval changes nothing.
func (c *PresenceConfig) Set(enabled bool, minutes int) error {
	minutes = interval(minutes)
	if !slices.Contains(AllowedPresenceIntervals, minutes) {
		return fmt.Errorf("%w: %d minutes", ErrBadPresenceInterval, minutes)
	}
	stored := minutes
	if minutes == DefaultPresenceInterval {
		stored = 0
	}
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.PresenceCheck, cfg.PresenceIntervalMinutes = enabled, stored
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the presence check settings: %w", err)
	}
	return nil
}

// Settings is what the check itself reads for each cycle: its switch, its interval and the declared scan ranges it sweeps.
func (c *PresenceConfig) Settings() PresenceSettings {
	cfg := c.store.Get()
	return PresenceSettings{
		Enabled: cfg.PresenceCheck,
		Every:   time.Duration(interval(cfg.PresenceIntervalMinutes)) * time.Minute,
		Targets: append([]string(nil), cfg.ScanRanges...),
	}
}
