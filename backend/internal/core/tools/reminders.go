package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ReminderSpec is a reminder as the agent asked for it. At is zero for a
// recurring reminder, which starts at its next cron occurrence.
type ReminderSpec struct {
	Message   string
	At        time.Time
	Cron      string
	Until     *time.Time
	Zone      string // IANA name, already resolved
	Connector string // channel type; empty = default channel
}

// ReminderView is a stored reminder as shown back to the agent.
type ReminderView struct {
	ID      int64
	Message string
	At      time.Time
	Cron    string
	Zone    string
	Channel string // where it will be delivered, or "in-app"
}

// ReminderService is what the tools need from the reminders subsystem. It is
// declared here, with its own types, so this package does not import it; the app
// layer adapts the concrete service.
type ReminderService interface {
	Schedule(ctx context.Context, spec ReminderSpec) (ReminderView, error)
	Cancel(ctx context.Context, id int64) error
	List(ctx context.Context) ([]ReminderView, error)
	// Zone resolves the requested IANA name, falling back to the owner's zone.
	Zone(requested string) (*time.Location, error)
}

// ReminderTools lets the agent schedule, list and cancel reminders.
type ReminderTools struct {
	svc ReminderService
}

// NewReminderTools returns the reminder tool provider.
func NewReminderTools(svc ReminderService) *ReminderTools { return &ReminderTools{svc: svc} }

// ScheduleReminderArgs are the schedule_reminder tool arguments.
type ScheduleReminderArgs struct {
	Message   string `json:"message"`
	When      string `json:"when"`
	Cron      string `json:"cron"`
	Until     string `json:"until"`
	Timezone  string `json:"timezone"`
	Connector string `json:"connector"`
}

// CancelReminderArgs are the cancel_reminder tool arguments.
type CancelReminderArgs struct {
	ID int64 `json:"id"`
}

const reminderTimeLayout = "Mon 2 Jan 2006 15:04"

// Schedule stores a reminder. The result states the resolved absolute time, zone
// and channel so the user can spot a wrong guess immediately.
func (t *ReminderTools) Schedule(ctx context.Context, args ScheduleReminderArgs) (any, error) {
	spec, err := t.parseSpec(args)
	if err != nil {
		return nil, err
	}
	view, err := t.svc.Schedule(ctx, spec)
	if err != nil {
		return nil, err
	}
	loc, _ := time.LoadLocation(view.Zone)
	if loc == nil {
		loc = time.UTC
	}
	when := view.At.In(loc).Format(reminderTimeLayout)
	if view.Cron != "" {
		return fmt.Sprintf("Reminder %d set: repeats (%s) starting %s %s via %s", view.ID, view.Cron, when, view.Zone, view.Channel), nil
	}
	return fmt.Sprintf("Reminder %d set for %s %s via %s", view.ID, when, view.Zone, view.Channel), nil
}

func (t *ReminderTools) parseSpec(args ScheduleReminderArgs) (ReminderSpec, error) {
	if strings.TrimSpace(args.Message) == "" {
		return ReminderSpec{}, errors.New("message is required: what should the reminder say?")
	}
	if (args.When == "") == (args.Cron == "") {
		return ReminderSpec{}, errors.New("give exactly one of 'when' (one-off) or 'cron' (recurring)")
	}
	loc, err := t.svc.Zone(strings.TrimSpace(args.Timezone))
	if err != nil {
		return ReminderSpec{}, err
	}
	spec := ReminderSpec{
		Message:   args.Message,
		Cron:      strings.TrimSpace(args.Cron),
		Zone:      loc.String(),
		Connector: strings.ToLower(strings.TrimSpace(args.Connector)),
	}
	if args.When != "" {
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(args.When))
		if err != nil {
			return ReminderSpec{}, fmt.Errorf("'when' must be RFC 3339 with an offset, e.g. 2026-10-12T09:00:00+01:00: %w", err)
		}
		spec.At = at.UTC()
	}
	if args.Until != "" {
		until, err := time.Parse(time.RFC3339, strings.TrimSpace(args.Until))
		if err != nil {
			return ReminderSpec{}, fmt.Errorf("'until' must be RFC 3339: %w", err)
		}
		u := until.UTC()
		spec.Until = &u
	}
	return spec, nil
}

// List describes the open reminders.
func (t *ReminderTools) List(ctx context.Context, _ struct{}) (any, error) {
	views, err := t.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(views) == 0 {
		return "No open reminders.", nil
	}
	var b strings.Builder
	for _, v := range views {
		loc, _ := time.LoadLocation(v.Zone)
		if loc == nil {
			loc = time.UTC
		}
		fmt.Fprintf(&b, "%d: %q at %s %s via %s", v.ID, v.Message, v.At.In(loc).Format(reminderTimeLayout), v.Zone, v.Channel)
		if v.Cron != "" {
			fmt.Fprintf(&b, " (repeats: %s)", v.Cron)
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String()), nil
}

// Cancel cancels an open reminder.
func (t *ReminderTools) Cancel(ctx context.Context, args CancelReminderArgs) (any, error) {
	if err := t.svc.Cancel(ctx, args.ID); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Reminder %d cancelled.", args.ID), nil
}
