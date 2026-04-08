package automation

import (
	"fmt"
	"llm-proxy/models"
	"time"
)

// IntervalTrigger fires on a fixed interval (e.g., "15m", "1h", "30s").
type IntervalTrigger struct {
	interval time.Duration
}

// NewIntervalTrigger creates an IntervalTrigger from a duration string.
func NewIntervalTrigger(value string) (*IntervalTrigger, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return nil, fmt.Errorf("invalid interval %q: %w", value, err)
	}
	if d <= 0 {
		return nil, fmt.Errorf("interval must be positive: %s", value)
	}
	return &IntervalTrigger{interval: d}, nil
}

func (t *IntervalTrigger) ShouldRun(lastRun, now time.Time) bool {
	if lastRun.IsZero() {
		return true
	}
	return now.Sub(lastRun) >= t.interval
}

func (t *IntervalTrigger) NextRun(now time.Time) time.Time {
	return now.Add(t.interval)
}

func (t *IntervalTrigger) Type() models.TriggerType {
	return models.TriggerInterval
}

func (t *IntervalTrigger) Value() string {
	return t.interval.String()
}
