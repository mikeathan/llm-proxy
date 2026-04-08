package automation

import (
	"fmt"
	"time"

	"llm-proxy/models"
)

// Trigger defines when an automation should run.
type Trigger interface {
	ShouldRun(lastRun, now time.Time) bool
	NextRun(now time.Time) time.Time
	Type() models.TriggerType
	Value() string
}

// Factory creates a Trigger from a TriggerConfig.
func New(cfg models.TriggerConfig) (Trigger, error) {
	switch cfg.Type {
	case models.TriggerCron:
		return NewCronTrigger(cfg.Value)
	case models.TriggerInterval:
		return NewIntervalTrigger(cfg.Value)
	case models.TriggerManual:
		return NewManualTrigger()
	default:
		return nil, fmt.Errorf("unknown trigger type: %q", cfg.Type)
	}
}
