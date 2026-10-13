package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/core/reminders"
	"llm-proxy/models"
)

const (
	remindName    = "remind"
	remindUsage   = "/remind <when> <message>"
	remindSummary = "Set a reminder: a day and time, then what to remind you of"
	createdByUser = "command"
	invalidPrefix = "invalid reminder: "
)

// remindExamples are shown by a UI that offers the command.
var remindExamples = []string{
	"tomorrow 9am call the bank",
	"sunday 8pm eye test monday 11:35",
	"18th of october 8pm eye test",
	"in 2h check the oven",
	"every weekday 9am stand-up",
}

// ReminderPlanner is what /remind needs from the reminders service: validate a
// reminder (Plan), store it (Create) and name the channel it will use.
type ReminderPlanner interface {
	Plan(r reminders.Reminder) (reminders.Reminder, error)
	Create(ctx context.Context, r reminders.Reminder) (reminders.Reminder, error)
	Channel(stored string) string
}

// Remind is the /remind command.
func Remind(svc ReminderPlanner) Command {
	c := remindCommand{svc: svc}
	return Command{
		Name: remindName, Usage: remindUsage, Summary: remindSummary, Examples: remindExamples,
		Preview: c.preview, Run: c.run,
	}
}

type remindCommand struct{ svc ReminderPlanner }

// plan reads the arguments into the reminder that would be stored.
func (c remindCommand) plan(ctx context.Context, req Request) (reminders.Reminder, error) {
	loc, err := time.LoadLocation(req.Zone)
	if err != nil {
		loc = time.UTC
	}
	parsed, err := reminders.ParseCommand(req.Args, req.Now, loc)
	if err != nil {
		return reminders.Reminder{}, err
	}
	planned, err := c.svc.Plan(reminders.Reminder{
		Message: parsed.Message, FireAt: parsed.At, Cron: parsed.Cron, Timezone: loc.String(),
		Origin: models.OriginConnector(ctx), CreatedBy: createdByUser,
	})
	if err != nil {
		return reminders.Reminder{}, readable(err)
	}
	return planned, nil
}

// readable drops the package's error prefix: the person typed a command, not a request body.
func readable(err error) error {
	if errors.Is(err, reminders.ErrInvalid) {
		return errors.New(strings.TrimPrefix(err.Error(), invalidPrefix))
	}
	return err
}

func (c remindCommand) preview(ctx context.Context, req Request) (string, error) {
	r, err := c.plan(ctx, req)
	if err != nil {
		return "", err
	}
	return c.describe(r), nil
}

func (c remindCommand) run(ctx context.Context, req Request) (string, error) {
	planned, err := c.plan(ctx, req)
	if err != nil {
		return "", err
	}
	got, err := c.svc.Create(ctx, planned)
	if err != nil {
		return "", readable(err)
	}
	return fmt.Sprintf("Reminder %d set for %s", got.ID, c.describe(got)), nil
}

// describe states the time, zone and channel so a wrong guess is visible.
func (c remindCommand) describe(r reminders.Reminder) string {
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		loc = time.UTC
	}
	when := r.FireAt.In(loc).Format(models.ReminderTimeLayout)
	channel := c.svc.Channel(r.Connector)
	if r.Cron != "" {
		return fmt.Sprintf("repeats (%s) from %s %s via %s: %s", r.Cron, when, loc, channel, r.Message)
	}
	return fmt.Sprintf("%s %s via %s: %s", when, loc, channel, r.Message)
}
