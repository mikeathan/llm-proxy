package trigger

import "time"

// ManualTrigger never fires on a schedule; only via explicit Trigger() call.
type ManualTrigger struct{}

func NewManualTrigger() (Trigger, error) {
	return &ManualTrigger{}, nil
}

func (t *ManualTrigger) ShouldRun(lastRun time.Time) bool {
	return false
}

func (t *ManualTrigger) NextRun() time.Time {
	return time.Time{} // Unset
}

func (t *ManualTrigger) Type() string {
	return "manual"
}
