package trigger

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// CronTrigger fires on a cron schedule.
type CronTrigger struct {
	schedule cron.Schedule
	parser   cron.Parser
}

// NewCronTrigger creates a CronTrigger from a standard cron expression.
// Supports seconds, minutes, hours, dom, month, dow, and descriptors.
func NewCronTrigger(expr string) (*CronTrigger, error) {
	parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return &CronTrigger{schedule: schedule, parser: parser}, nil
}

func (t *CronTrigger) ShouldRun(lastRun time.Time) bool {
	if lastRun.IsZero() {
		return true
	}
	return t.schedule.Next(lastRun).Before(time.Now()) || t.schedule.Next(lastRun).Equal(time.Now())
}

func (t *CronTrigger) NextRun() time.Time {
	return t.schedule.Next(time.Now())
}

func (t *CronTrigger) Type() string {
	return "cron"
}
