package app

import (
	"fmt"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/commands"
)

// buildCommands assembles the slash commands. /remind exists only when reminders
// do, /research and /agent only when the agent catalog does, /run only when the
// dispatcher does; the registry is never nil, so a channel can always ask it.
func buildCommands(s *AppServices, disp *automation.Dispatcher) *commands.Registry {
	var cmds []commands.Command
	if s.Reminders != nil {
		cmds = append(cmds, commands.Remind(s.Reminders))
	}
	if catalog := s.Profiles(); catalog != nil {
		agents := commands.Agents{Catalog: catalog, HelpersOff: func() string { return helpersOff(s) }}
		cmds = append(cmds, commands.Research(agents), commands.Agent(agents))
	}
	if disp != nil {
		cmds = append(cmds, commands.Run(automationStarter{disp}))
	}
	return commands.New(cmds...)
}

// automationStarter is /run's way into the dispatcher: a manual trigger, the same one the Run button uses. Trigger is
// a cheap lane enqueue (the run timeout starts at dequeue), so the command calls it inline; the channel follows the
// run's progress on the automation's event stream.
type automationStarter struct{ disp *automation.Dispatcher }

func (a automationStarter) Start(workspaceID, name string) (commands.AutomationStart, error) {
	result, err := a.disp.Trigger(workspaceID, name, "")
	if err != nil {
		return commands.AutomationStart{}, err
	}
	return commands.AutomationStart{Status: startStatus(result.Status), Position: result.Position}, nil
}

// startStatus is the dispatcher's trigger status as /run reads it.
func startStatus(status string) string {
	switch status {
	case automation.TriggerQueued:
		return commands.AutomationQueued
	case automation.TriggerSkipped:
		return commands.AutomationSkipped
	default:
		return commands.AutomationStarted
	}
}

// helpersOff says, in the words the chat's settings use, why helpers cannot run now ("" when they can).
func helpersOff(s *AppServices) string {
	problem := assistant.HelperUnset
	if runner := s.SubRunner(); runner != nil {
		problem = runner.Problem()
	}
	return helperProblemText(problem, s.AgentSettings().SubagentModelName())
}

// helperProblemText is the reason a helper problem gives the person.
func helperProblemText(problem assistant.HelperProblem, model string) string {
	switch problem {
	case assistant.HelperNone:
		return ""
	case assistant.HelperMissing:
		return fmt.Sprintf("the Helper model %s is no longer in your models", model)
	case assistant.HelperNotCloud:
		return fmt.Sprintf("the Helper model %s is not a cloud model", model)
	default:
		return "no Helper model is chosen in Settings → Runs"
	}
}
