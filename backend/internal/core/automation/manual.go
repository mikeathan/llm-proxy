package automation

import (
	"llm-proxy/models"
	"time"
)

// ManualTrigger never fires on a schedule; only via explicit Trigger() call.
type ManualTrigger struct{}

func NewManualTrigger() (Trigger, error) {
	return &ManualTrigger{}, nil
}

func (t *ManualTrigger) ShouldRun(lastRun, now time.Time) bool {
	return false
}

func (t *ManualTrigger) NextRun(now time.Time) time.Time {
	return time.Time{} // Unset
}

func (t *ManualTrigger) Type() models.TriggerType {
	return models.TriggerManual
}

func (t *ManualTrigger) Value() string {
	return ""
}
