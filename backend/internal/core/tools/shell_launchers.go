package tools

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"

	"llm-proxy/models"

	"mvdan.cc/sh/v3/syntax"
)

// shellLaunchers are interpreters that run a shell program given as an argument
// (-c), as a file, or on stdin. `eval` is the same thing inside the current shell.
var shellLaunchers = map[string]bool{"bash": true, "sh": true, "dash": true, "zsh": true, "ksh": true}

// launcherInput is where a shell launcher gets the program it runs.
type launcherInput int

const (
	inputScriptArg  launcherInput = iota // -c 'program'
	inputFile                            // a script file: written by the agent, outside this check
	inputStdin                           // piped or typed in: cannot be read here
	inputNone                            // --version / --help: runs nothing
	inputUnreadable                      // a script operand that is not a literal (`bash $X`, `bash <(cmd)`)
)

// stdinPaths name the shell's own stdin: `bash /dev/stdin` reads commands from it.
var stdinPaths = []string{"/dev/stdin", "/dev/fd/0", "/proc/self/fd/0"}

// validateShellLaunchers re-validates the program a shell launcher or `eval`
// runs: `bash -c '<disallowed>'` would otherwise bypass the whitelist. The inner
// command is validated with the full pipeline (patterns, paths, whitelist, path
// security). A program that is not a literal, or that arrives on stdin, cannot be
// read: with an allowed list that is refused, without one there is nothing to
// check its names against.
func validateShellLaunchers(commands []shellCommand, whitelisted bool, cfg models.TerminalGuardrailsConfig, blocked []string, cache *sync.Map, jailPath, cwd string) error {
	for _, cmd := range commands {
		program, err := launchedProgram(cmd)
		if err != nil && whitelisted {
			return fmt.Errorf("shell command rejected: %w", err)
		}
		if err != nil || program == "" {
			continue
		}
		if err := ValidateTerminalCommand(program, cfg, blocked, cache, jailPath, cwd); err != nil {
			return fmt.Errorf("shell -c command rejected: %w", err)
		}
	}
	return nil
}

// launchedProgram is the shell program a command runs from its arguments, or ""
// when it runs none that can be read from here (not a launcher, a script file).
func launchedProgram(cmd shellCommand) (string, error) {
	name := path.Base(cmd.name)
	if name == "eval" {
		return evalProgram(cmd.args)
	}
	if !shellLaunchers[name] {
		return "", nil
	}
	input, script := launcherSource(cmd.args)
	switch input {
	case inputStdin:
		return "", fmt.Errorf("%s reads its commands from stdin, so they cannot be checked", name)
	case inputUnreadable:
		return "", fmt.Errorf("the script %s runs is not a literal, so it cannot be checked", name)
	case inputScriptArg:
		program, literal := literalWord(script)
		if !literal {
			return "", fmt.Errorf("the -c argument of %s is not a literal, so it cannot be checked", name)
		}
		return program, nil
	}
	return "", nil
}

// evalProgram is the text `eval` runs: its literal arguments joined by spaces.
func evalProgram(args []*syntax.Word) (string, error) {
	parts := make([]string, len(args))
	for i, arg := range args {
		part, literal := literalWord(arg)
		if !literal {
			return "", fmt.Errorf("an eval argument is not a literal, so what it runs cannot be checked")
		}
		parts[i] = part
	}
	return strings.Join(parts, " "), nil
}

// launcherOption is what one argument of a shell launcher means.
type launcherOption int

const (
	optPlain      launcherOption = iota // a flag that changes nothing here (-l, -x, --norc)
	optOperand                          // the first word that is not an option: the script file
	optDashDash                         // --: the next word is the script file
	optInfo                             // --version, --help
	optStdin                            // -s or -: read commands from stdin
	optScript                           // -c (alone or bundled: -lc, -ec): the next word is the program
	optTakesValue                       // -o posix, -O extglob, --rcfile F: the next word is its value
	optUnreadable                       // a word that is not a literal, where an option or script file goes
)

var launcherWords = map[string]launcherOption{
	"-": optStdin, "--": optDashDash, "--version": optInfo, "--help": optInfo,
	"--rcfile": optTakesValue, "--init-file": optTakesValue,
}

func classifyLauncherOption(opt string, literal bool) launcherOption {
	if !literal {
		return optUnreadable
	}
	if !strings.HasPrefix(opt, "-") && !strings.HasPrefix(opt, "+") {
		return optOperand
	}
	if kind, ok := launcherWords[opt]; ok {
		return kind
	}
	if strings.HasPrefix(opt, "--") {
		return optPlain
	}
	return classifyShortOptions(opt)
}

// classifyShortOptions reads a bundle of one-letter options (-lc, -ec, +x: a + bundle
// switches the same options off, and still leaves the shell reading stdin).
func classifyShortOptions(opt string) launcherOption {
	switch {
	case strings.Contains(opt, "s"):
		return optStdin
	case strings.Contains(opt, "c"):
		return optScript
	case strings.ContainsAny(opt, "oO"):
		return optTakesValue
	}
	return optPlain
}

// launcherSource finds where a shell launcher's program comes from. With none of
// a -c program, a script file or an informational flag, the shell reads stdin.
func launcherSource(args []*syntax.Word) (launcherInput, *syntax.Word) {
	for i := 0; i < len(args); i++ {
		opt, literal := literalWord(args[i])
		switch classifyLauncherOption(opt, literal) {
		case optUnreadable:
			return inputUnreadable, nil
		case optOperand:
			if slices.Contains(stdinPaths, opt) {
				return inputStdin, nil
			}
			return inputFile, nil
		case optStdin:
			return inputStdin, nil
		case optDashDash:
			if i+1 < len(args) {
				return inputFile, nil
			}
			return inputStdin, nil
		case optInfo:
			return inputNone, nil
		case optScript:
			if i+1 < len(args) {
				return inputScriptArg, args[i+1]
			}
			return inputNone, nil
		case optTakesValue:
			i++
		}
	}
	return inputStdin, nil
}
