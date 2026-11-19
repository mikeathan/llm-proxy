package reminders

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"llm-proxy-stacks/sdk"
)

// The tools the stack declares. They are called by the core on behalf of the assistant (schedule, list, cancel) and of the /remind command: the
// core checks who is asking, adds what only it knows (the connector the message arrived through, the zone the conversation reports) from its own
// context and never from the model's arguments, and sends back what the stack answers. The answer is the sentence the person is shown.
const (
	ToolSchedule = "reminders_schedule"
	ToolList     = "reminders_list"
	ToolCancel   = "reminders_cancel"
	ToolCommand  = "reminders_command"
)

// ScheduleArgs are what the core sends to reminders_schedule: the assistant's arguments plus the two facts the core adds.
type ScheduleArgs struct {
	Message   string `json:"message"`
	When      string `json:"when"`
	Cron      string `json:"cron"`
	Until     string `json:"until"`
	Timezone  string `json:"timezone"`
	Connector string `json:"connector"`
	// ChatZone is the zone the conversation reports; it is used when the assistant named none.
	ChatZone string `json:"chat_zone,omitempty"`
	// Origin is the connector type the request arrived through, "" for the web chat.
	Origin string `json:"origin,omitempty"`
}

// CancelArgs are what the core sends to reminders_cancel.
type CancelArgs struct {
	ID int64 `json:"id"`
}

// CommandArgs are what the core sends to reminders_command: the text after /remind, the person's zone, the time the command was typed at and
// whether to only describe what would be set.
type CommandArgs struct {
	Text    string    `json:"text"`
	Zone    string    `json:"zone"`
	Now     time.Time `json:"now"`
	Origin  string    `json:"origin,omitempty"`
	Preview bool      `json:"preview,omitempty"`
}

const (
	createdByAssistant = "assistant"
	createdByCommand   = "command"
	invalidPrefix      = "invalid reminder: "
)

// tools answers the declared tools over the service. zone resolves a zone name, falling back to the owner's.
type tools struct {
	svc  *Service
	zone func(requested string) (*time.Location, error)
}

// call runs one tool and returns its answer as a JSON string. A refusal the person should read is an *Error with CodeInvalid.
func (t tools) call(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	var text string
	var err error
	switch name {
	case ToolSchedule:
		text, err = t.schedule(ctx, args)
	case ToolList:
		text, err = t.list(ctx)
	case ToolCancel:
		text, err = t.cancel(ctx, args)
	case ToolCommand:
		text, err = t.command(ctx, args)
	default:
		return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no tool "+name)
	}
	if err != nil {
		return nil, toolError(err)
	}
	return json.Marshal(text)
}

// toolError gives a failure its code. The text is kept as it was said before the stack existed, so the person and the model read the same words.
func toolError(err error) error {
	var coded *stacksdk.Error
	switch {
	case errors.As(err, &coded):
		return err
	case errors.Is(err, ErrOff):
		return stacksdk.Errorf(stacksdk.CodeUnavailable, err.Error())
	case errors.Is(err, ErrNotFound):
		return stacksdk.Errorf(stacksdk.CodeNotFound, err.Error())
	case errors.Is(err, ErrInvalid), isArgError(err):
		return stacksdk.Errorf(stacksdk.CodeInvalid, err.Error())
	}
	return stacksdk.Errorf(stacksdk.CodeInternal, err.Error())
}

// argError is a refusal of what the assistant or the person asked for (not a storage failure).
type argError struct{ msg string }

func (e argError) Error() string { return e.msg }

func isArgError(err error) bool {
	var a argError
	return errors.As(err, &a)
}

func decodeArgs(args json.RawMessage, v any) error {
	if err := json.Unmarshal(args, v); err != nil {
		return argError{"the arguments could not be read: " + err.Error()}
	}
	return nil
}

// schedule stores a reminder. The result states the resolved absolute time, zone and channel so the user can spot a wrong guess immediately.
func (t tools) schedule(ctx context.Context, raw json.RawMessage) (string, error) {
	var a ScheduleArgs
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	spec, err := t.parse(a)
	if err != nil {
		return "", err
	}
	spec.Origin, spec.CreatedBy = a.Origin, createdByAssistant
	got, err := t.svc.Create(ctx, spec)
	if err != nil {
		return "", err
	}
	loc, _ := time.LoadLocation(got.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	when := got.FireAt.In(loc).Format(ReminderTimeLayout)
	channel := t.svc.Channel(got.Connector)
	if got.Cron != "" {
		return fmt.Sprintf("Reminder %d set: repeats (%s) starting %s %s via %s", got.ID, got.Cron, when, got.Timezone, channel), nil
	}
	return fmt.Sprintf("Reminder %d set for %s %s via %s", got.ID, when, got.Timezone, channel), nil
}

func (t tools) parse(a ScheduleArgs) (Reminder, error) {
	if strings.TrimSpace(a.Message) == "" {
		return Reminder{}, argError{"message is required: what should the reminder say?"}
	}
	if (a.When == "") == (a.Cron == "") {
		return Reminder{}, argError{"give exactly one of 'when' (one-off) or 'cron' (recurring)"}
	}
	// A zone the user named wins; otherwise the conversation's own zone, so "8pm" means their 8pm.
	loc, err := t.zone(cmp.Or(strings.TrimSpace(a.Timezone), a.ChatZone))
	if err != nil {
		return Reminder{}, argError{err.Error()}
	}
	r := Reminder{Message: a.Message, Cron: strings.TrimSpace(a.Cron), Timezone: loc.String(), Connector: strings.ToLower(strings.TrimSpace(a.Connector))}
	if a.When != "" {
		at, err := ParseInstantIn("when", a.When, loc)
		if err != nil {
			return Reminder{}, argError{err.Error()}
		}
		r.FireAt = at
	}
	if a.Until != "" {
		until, err := ParseInstantIn("until", a.Until, loc)
		if err != nil {
			return Reminder{}, argError{err.Error()}
		}
		r.Until = &until
	}
	return r, nil
}

// list describes the open reminders.
func (t tools) list(ctx context.Context) (string, error) {
	open, err := t.svc.List(ctx)
	if err != nil {
		return "", err
	}
	if len(open) == 0 {
		return "No open reminders.", nil
	}
	var b strings.Builder
	for _, r := range open {
		loc, _ := time.LoadLocation(r.Timezone)
		if loc == nil {
			loc = time.UTC
		}
		fmt.Fprintf(&b, "%d: %q at %s %s via %s", r.ID, r.Message, r.FireAt.In(loc).Format(ReminderTimeLayout), r.Timezone, t.svc.Channel(r.Connector))
		if r.Cron != "" {
			fmt.Fprintf(&b, " (repeats: %s)", r.Cron)
		}
		if r.Status == StatusPaused {
			b.WriteString(" (paused)")
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String()), nil
}

// cancel cancels an open reminder.
func (t tools) cancel(ctx context.Context, raw json.RawMessage) (string, error) {
	var a CancelArgs
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	if err := t.svc.Cancel(ctx, a.ID); err != nil {
		return "", err
	}
	return fmt.Sprintf("Reminder %d cancelled.", a.ID), nil
}

// command is /remind: it reads the person's text into a reminder, and either describes it (a preview) or stores it.
func (t tools) command(ctx context.Context, raw json.RawMessage) (string, error) {
	var a CommandArgs
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	loc, err := time.LoadLocation(a.Zone)
	if err != nil {
		loc = time.UTC
	}
	parsed, err := ParseCommand(a.Text, a.Now, loc)
	if err != nil {
		return "", argError{err.Error()}
	}
	planned, err := t.svc.Plan(Reminder{Message: parsed.Message, FireAt: parsed.At, Cron: parsed.Cron, Timezone: loc.String(), Origin: a.Origin, CreatedBy: createdByCommand})
	if err != nil {
		return "", readable(err)
	}
	if a.Preview {
		return t.describe(planned), nil
	}
	got, err := t.svc.Create(ctx, planned)
	if err != nil {
		return "", readable(err)
	}
	return fmt.Sprintf("Reminder %d set for %s", got.ID, t.describe(got)), nil
}

// readable drops the package's error prefix: the person typed a command, not a request body.
func readable(err error) error {
	if errors.Is(err, ErrInvalid) {
		return argError{strings.TrimPrefix(err.Error(), invalidPrefix)}
	}
	return err
}

// describe states the time, zone and channel so a wrong guess is visible.
func (t tools) describe(r Reminder) string {
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		loc = time.UTC
	}
	when := r.FireAt.In(loc).Format(ReminderTimeLayout)
	channel := t.svc.Channel(r.Connector)
	if r.Cron != "" {
		return fmt.Sprintf("repeats (%s) from %s %s via %s: %s", r.Cron, when, loc, channel, r.Message)
	}
	return fmt.Sprintf("%s %s via %s: %s", when, loc, channel, r.Message)
}
