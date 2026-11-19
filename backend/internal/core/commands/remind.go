package commands

import (
	"context"
	"time"

	"llm-proxy/models"
)

const (
	remindName    = "remind"
	remindUsage   = "/remind <when> <message>"
	remindSummary = "Set a reminder: a day and time, then what to remind you of"
)

// remindExamples are shown by a UI that offers the command.
var remindExamples = []string{
	"tomorrow 9am call the bank",
	"sunday 8pm eye test monday 11:35",
	"18th of october 8pm eye test",
	"in 2h check the oven",
	"every weekday 9am stand-up",
}

// RemindRequest is what /remind sends to the Reminders stack: the person's text, the zone their times are in, when it was typed and the connector
// it arrived through. Preview asks only for the description of what would be set.
type RemindRequest struct {
	Text    string
	Zone    string
	Now     time.Time
	Origin  string
	Preview bool
}

// Reminders is what /remind needs from the Reminders stack (ADR-0068): it reads the text into a reminder, stores it or only describes it, and answers
// with the sentence to show, or with the refusal in plain words. The stack does the reading, so the words it understands live in one place.
type Reminders interface {
	Remind(ctx context.Context, req RemindRequest) (string, error)
}

// Remind is the /remind command.
func Remind(svc Reminders) Command {
	c := remindCommand{svc: svc}
	return Command{
		Name: remindName, Usage: remindUsage, Summary: remindSummary, Examples: remindExamples,
		Preview: c.preview, Run: c.run,
	}
}

type remindCommand struct{ svc Reminders }

func (c remindCommand) request(ctx context.Context, req Request, preview bool) RemindRequest {
	return RemindRequest{Text: req.Args, Zone: req.Zone, Now: req.Now, Origin: models.OriginConnector(ctx), Preview: preview}
}

func (c remindCommand) preview(ctx context.Context, req Request) (string, error) {
	return c.svc.Remind(ctx, c.request(ctx, req, true))
}

func (c remindCommand) run(ctx context.Context, req Request) (string, error) {
	return c.svc.Remind(ctx, c.request(ctx, req, false))
}
