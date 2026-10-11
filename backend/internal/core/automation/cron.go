package automation

import (
	"fmt"
	"llm-proxy/models"
	"time"

	"github.com/robfig/cron/v3"
)

// cronParser is the one cron grammar in the service: five fields (minute, hour,
// day of month, month, day of week) or a descriptor such as @daily or
// @every 1h. Seconds are deliberately not accepted, so a mistyped
// six-field expression is rejected instead of firing every few seconds. The
// trigger, the dispatcher's scheduler and the schedule preview all use it, so
// they cannot disagree about what a schedule means.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// CronTrigger fires on a cron schedule.
type CronTrigger struct {
	schedule cron.Schedule
	expr     string
}

// NewCronTrigger creates a CronTrigger from a five-field cron expression or a
// descriptor.
func NewCronTrigger(expr string) (*CronTrigger, error) {
	schedule, err := cronParser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return &CronTrigger{schedule: schedule, expr: expr}, nil
}

func (t *CronTrigger) ShouldRun(lastRun, now time.Time) bool {
	if lastRun.IsZero() {
		return true
	}
	return t.schedule.Next(lastRun).Before(now) || t.schedule.Next(lastRun).Equal(now)
}

func (t *CronTrigger) NextRun(now time.Time) time.Time {
	return t.schedule.Next(now)
}

func (t *CronTrigger) Type() models.TriggerType {
	return models.TriggerCron
}

func (t *CronTrigger) Value() string {
	return t.expr
}
