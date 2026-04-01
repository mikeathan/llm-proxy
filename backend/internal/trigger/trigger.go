package trigger

import (
	"fmt"
	"time"

	"llm-proxy/models"
)

// Trigger is a pure interface that evaluates whether an automation should run.
// ShouldRun is a pure predicate with no side effects.
type Trigger interface {
	ShouldRun(lastRun time.Time) bool
	NextRun() time.Time
	Type() string // "cron" | "interval" | "manual"
}

// Factory creates a Trigger from a TriggerConfig.
func New(cfg models.TriggerConfig) (Trigger, error) {
	switch cfg.Type {
	case "cron":
		return NewCronTrigger(cfg.Value)
	case "interval":
		return NewIntervalTrigger(cfg.Value)
	case "manual":
		return NewManualTrigger()
	default:
		return nil, fmt.Errorf("unknown trigger type: %q", cfg.Type)
	}
}
