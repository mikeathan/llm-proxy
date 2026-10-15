package tools

import (
	"errors"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// The terminal guardrails judge the commands a shell would run, so they read the
// command the way the shell does: they parse it (the executor is bash) and walk
// the tree. Splitting the text at `;`, `&&` and newlines and reading the first
// word of each piece cannot do that: it took `for`, `do` and `done` for
// programs, and it never saw a command inside `$(...)`, backticks, `<(...)`, a
// loop body after `do`, or a job after a single `&`.

// shellCommand is one simple command found anywhere in a parsed command line.
type shellCommand struct {
	name string
	args []*syntax.Word
}

// parseShell parses a command as bash does. Anything the shell would reject
// (an unterminated quote or heredoc, a stray `fi`) is an error, so a command
// that cannot be read is never run.
func parseShell(command string) (*syntax.File, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, fmt.Errorf("command is not valid shell syntax: %w", err)
	}
	return file, nil
}

// errUnreadable marks a command that parses but whose commands cannot all be known
// before it runs (a name held in a variable, a shell fed on stdin). Whether that
// matters is the caller's: it only does when there is an allowed list to check
// names against.
var errUnreadable = errors.New("what it runs cannot be checked")

// shellCommands returns every simple command the shell would execute, and what
// the wrappers among them launch (shell_wrappers.go): those in pipelines and
// lists, in loop and branch bodies, subshells and groups, function bodies,
// command substitutions, process substitutions and unquoted heredoc bodies.
// Assignments alone (`FOO=1`) run nothing and are not commands.
//
// A command the shell would reject returns that parse error and no commands. One
// whose name is not a literal (`$CMD`, `$(echo rm)`) or whose wrapper cannot be
// read returns the commands it could read together with an error wrapping
// errUnreadable, so a caller with an allowed list can fail closed.
func shellCommands(command string) ([]shellCommand, error) {
	file, err := parseShell(command)
	if err != nil {
		return nil, err
	}
	var found []shellCommand
	var unreadable error
	note := func(err error) {
		if unreadable == nil {
			unreadable = fmt.Errorf("%w: %w", errUnreadable, err)
		}
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		name, literal := literalWord(call.Args[0])
		if !literal {
			note(fmt.Errorf("command name %q is not a literal", wordSource(command, call.Args[0])))
			return true
		}
		cmd := shellCommand{name: name, args: call.Args[1:]}
		found = append(found, cmd)
		launched, err := launchedCommands(cmd)
		if err != nil {
			note(err)
		}
		found = append(found, launched...)
		return true
	})
	return found, unreadable
}

// ShellCommandNames returns the name of every command the shell would run, in
// order, for the allowed-command list.
func ShellCommandNames(command string) ([]string, error) {
	cmds, err := shellCommands(command)
	if err != nil {
		return nil, err // an unreadable command saves nothing
	}
	return commandNames(cmds), nil
}

func commandNames(cmds []shellCommand) []string {
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = c.name
	}
	return names
}

// literalWord returns a word's text when it is fixed at parse time: bare text or
// quoted text with no expansion in it. Anything with a variable, substitution,
// arithmetic or glob-free expansion is not literal.
func literalWord(word *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return "", false
				}
				b.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return b.String(), b.Len() > 0
}

// wordSource is the text of a word as written, for error messages.
func wordSource(command string, word *syntax.Word) string {
	start, end := int(word.Pos().Offset()), int(word.End().Offset())
	if start < 0 || end > len(command) || start >= end {
		return "?"
	}
	return command[start:end]
}
