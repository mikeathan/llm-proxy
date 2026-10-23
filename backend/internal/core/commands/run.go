package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	runName  = "run"
	runUsage = "/run <automation>"
)

// The statuses an AutomationStart reports.
const (
	AutomationStarted = "started"
	AutomationQueued  = "queued"
	AutomationSkipped = "skipped"
)

// AutomationStart is what asking an automation to run now did: it started, it is queued behind other work (Position),
// or it was skipped (a heartbeat with nothing to check).
type AutomationStart struct {
	Status   string
	Position int
}

// AutomationStarter runs one of a workspace's automations now, by name.
type AutomationStarter interface {
	Start(workspaceID, name string) (AutomationStart, error)
}

// Run is the /run command: run one of the workspace's automations now. The channel it came from may follow the run
// and send its result (Reply.Automation).
func Run(starter AutomationStarter) Command {
	name := func(req Request) (string, error) {
		automation := strings.TrimSpace(req.Args)
		switch {
		case automation == "":
			return "", errors.New("name an automation to run")
		case req.WorkspaceID == "":
			return "", errors.New("this chat has no workspace to run an automation in")
		}
		return automation, nil
	}
	return Command{
		Name:     runName,
		Usage:    runUsage,
		Summary:  "Run one of this workspace's automations now",
		Examples: []string{"llm_news"},
		Preview: func(_ context.Context, req Request) (string, error) {
			automation, err := name(req)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Runs '%s' now", automation), nil
		},
		Start: func(_ context.Context, req Request) (Started, error) {
			automation, err := name(req)
			if err != nil {
				return Started{}, err
			}
			start, err := starter.Start(req.WorkspaceID, automation)
			if err != nil {
				return Started{}, fmt.Errorf("could not run %q: %w", automation, err)
			}
			return started(automation, start), nil
		},
	}
}

// started is the line a run's start reads as, and whether there is a run to follow.
func started(automation string, start AutomationStart) Started {
	switch start.Status {
	case AutomationQueued:
		return Started{Text: fmt.Sprintf("'%s' is queued (position %d).", automation, start.Position), Automation: automation}
	case AutomationSkipped:
		return Started{Text: fmt.Sprintf("'%s' was skipped: it has nothing to run now.", automation)}
	default:
		return Started{Text: fmt.Sprintf("Running '%s'...", automation), Automation: automation}
	}
}
