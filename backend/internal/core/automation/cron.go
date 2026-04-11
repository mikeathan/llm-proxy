package automation

import (
	"fmt"
	"llm-proxy/models"
	"time"

	"github.com/robfig/cron/v3"
)

// CronTrigger fires on a cron schedule.
type CronTrigger struct {
	schedule cron.Schedule
	parser   cron.Parser
	expr     string
}

// NewCronTrigger creates a CronTrigger from a standard cron expression.
// Supports seconds, minutes, hours, dom, month, dow, and descriptors.
func NewCronTrigger(expr string) (*CronTrigger, error) {
	parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return &CronTrigger{schedule: schedule, parser: parser, expr: expr}, nil
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
