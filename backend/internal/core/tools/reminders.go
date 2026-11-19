package tools

import (
	"context"

	"llm-proxy/models"
)

// ReminderRequest is what the assistant asked for, with the two facts only the core knows: the zone the conversation reports and the connector
// the message arrived through. The core adds them from its own context, never from the model's arguments.
type ReminderRequest struct {
	Message   string
	When      string
	Cron      string
	Until     string
	Timezone  string
	Connector string
	// ChatZone is used when the assistant named no zone, so "8pm" means the person's 8pm.
	ChatZone string
	// Origin is the connector type the request arrived through; empty for the web chat.
	Origin string
}

// ReminderService is what the tools need from the Reminders stack (ADR-0068). It is declared here, with its own types, so this package imports
// neither the stack nor the SDK; the app layer adapts the hosted stack. Each call answers with the sentence the person is shown (the stack
// resolves the time, the zone and the channel and states them, so a wrong guess is visible at once), or with the refusal in the stack's words.
type ReminderService interface {
	Schedule(ctx context.Context, req ReminderRequest) (string, error)
	List(ctx context.Context) (string, error)
	Cancel(ctx context.Context, id int64) (string, error)
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

// Schedule stores a reminder. The result states the resolved absolute time, zone and channel so the user can spot a wrong guess immediately.
func (t *ReminderTools) Schedule(ctx context.Context, args ScheduleReminderArgs) (any, error) {
	out, err := t.svc.Schedule(ctx, ReminderRequest{
		Message: args.Message, When: args.When, Cron: args.Cron, Until: args.Until, Timezone: args.Timezone, Connector: args.Connector,
		ChatZone: models.ChatTimezone(ctx), Origin: models.OriginConnector(ctx),
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// List describes the open reminders.
func (t *ReminderTools) List(ctx context.Context, _ struct{}) (any, error) {
	out, err := t.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Cancel cancels an open reminder.
func (t *ReminderTools) Cancel(ctx context.Context, args CancelReminderArgs) (any, error) {
	out, err := t.svc.Cancel(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	return out, nil
}
