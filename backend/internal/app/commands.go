package app

import "llm-proxy/internal/core/commands"

// buildCommands assembles the slash commands. /remind exists only when reminders
// do; the registry is never nil, so a channel can always ask it.
func buildCommands(s *AppServices) *commands.Registry {
	var cmds []commands.Command
	if s.Reminders != nil {
		cmds = append(cmds, commands.Remind(s.Reminders))
	}
	return commands.New(cmds...)
}
