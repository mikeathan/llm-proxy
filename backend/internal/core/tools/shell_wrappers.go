package tools

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// A wrapper runs another program given as an argument (`env rm x`, `xargs rm`,
// `find -exec rm {} \;`). Allowing the wrapper must not allow what it launches,
// so the walker adds each launched command to the commands under test, however
// deeply wrapped. The table is deliberately narrow: an option it does not know,
// a non-literal option and a missing command all fail closed.

// optionSpec says which options a wrapper takes before the command it launches.
type optionSpec struct {
	flags     []string // options without a value
	valueOpts []string // options followed by a value (-n 5, -n5, --max-args=5, --max-args 5)
	numeric   bool     // a bare -N is a flag (nice -5)
	skip      int      // positional words between the options and the command (timeout's duration)
	optional  bool     // the command may be missing: `env` prints the environment, `exec 2>&1` only redirects
}

var numericOption = regexp.MustCompile(`^-[0-9]+$`)
var assignmentWord = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// A wrapper reads its own arguments and returns the commands it launches.
type wrapper func(args []*syntax.Word) ([]shellCommand, error)

var shellWrappers = map[string]wrapper{
	"env":     unwrapEnv,
	"nohup":   unwrapWith(optionSpec{}),
	"nice":    unwrapWith(optionSpec{valueOpts: []string{"-n", "--adjustment"}, numeric: true}),
	"timeout": unwrapWith(optionSpec{flags: []string{"-v", "--verbose", "--foreground", "--preserve-status"}, valueOpts: []string{"-s", "-k", "--signal", "--kill-after"}, skip: 1}),
	"command": unwrapCommandBuiltin,
	"builtin": unwrapWith(optionSpec{}),
	"exec":    unwrapWith(optionSpec{flags: []string{"-c", "-l"}, valueOpts: []string{"-a"}, optional: true}),
	"sudo": unwrapWith(optionSpec{
		flags:     []string{"-n", "-E", "-H", "-S", "-b", "-k", "--non-interactive", "--preserve-env", "--stdin"},
		valueOpts: []string{"-u", "-g", "-D", "-R", "-T", "--user", "--group"},
	}),
	"xargs": unwrapXargs,
	"find":  unwrapFind,
}

var xargsSpec = optionSpec{
	flags: []string{"-0", "-r", "-t", "-p", "-x", "-o", "-i", "--null", "--no-run-if-empty", "--verbose", "--interactive", "--open-tty", "--exit"},
	valueOpts: []string{"-I", "-n", "-P", "-L", "-d", "-s", "-a", "-E",
		"--max-args", "--max-procs", "--max-lines", "--delimiter", "--max-chars", "--arg-file", "--eof", "--replace"},
}

var envSpec = optionSpec{
	flags:     []string{"-", "-i", "-0", "-v", "--ignore-environment", "--null", "--debug"},
	valueOpts: []string{"-u", "-C", "--unset", "--chdir"},
}

// launchedCommands returns every command a command launches through wrappers:
// what a wrapper starts, and what that starts in turn.
func launchedCommands(cmd shellCommand) ([]shellCommand, error) {
	var all []shellCommand
	pending := []shellCommand{cmd}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		unwrap, ok := shellWrappers[path.Base(current.name)]
		if !ok {
			continue
		}
		inner, err := unwrap(current.args)
		if err != nil {
			return all, fmt.Errorf("%s: %w", current.name, err)
		}
		all = append(all, inner...)
		pending = append(pending, inner...)
	}
	return all, nil
}

func unwrapWith(spec optionSpec) wrapper {
	return func(args []*syntax.Word) ([]shellCommand, error) {
		rest, err := skipOptions(args, spec)
		if err != nil || (spec.optional && len(rest) == 0) {
			return nil, err
		}
		return launchedFrom(rest, "")
	}
}

// unwrapCommandBuiltin: `command -v rm` only looks a name up and runs nothing.
func unwrapCommandBuiltin(args []*syntax.Word) ([]shellCommand, error) {
	if len(args) > 0 {
		if opt, ok := literalWord(args[0]); ok && (opt == "-v" || opt == "-V") {
			return nil, nil
		}
	}
	return unwrapWith(optionSpec{flags: []string{"-p"}, optional: true})(args)
}

// unwrapEnv: options, then NAME=VALUE words, then the command. -S/--split-string
// runs a string as a command line, which is not readable here, so it is refused.
func unwrapEnv(args []*syntax.Word) ([]shellCommand, error) {
	rest, err := skipOptions(args, envSpec)
	if err != nil {
		return nil, err
	}
	for len(rest) > 0 && isAssignmentWord(rest[0]) {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return nil, nil // plain `env` prints the environment
	}
	return launchedFrom(rest, "")
}

func unwrapXargs(args []*syntax.Word) ([]shellCommand, error) {
	rest, err := skipOptions(args, xargsSpec)
	if err != nil {
		return nil, err
	}
	return launchedFrom(rest, "echo") // xargs with no command runs echo
}

// unwrapFind: the word after -exec, -execdir, -ok or -okdir is a command.
func unwrapFind(args []*syntax.Word) ([]shellCommand, error) {
	var launched []shellCommand
	for i, arg := range args {
		opt, literal := literalWord(arg)
		if !literal || !isFindExec(opt) {
			continue
		}
		inner, err := launchedFrom(args[i+1:], "")
		if err != nil {
			return nil, err
		}
		launched = append(launched, inner...)
	}
	return launched, nil
}

func isFindExec(opt string) bool {
	switch opt {
	case "-exec", "-execdir", "-ok", "-okdir":
		return true
	}
	return false
}

// launchedFrom reads the command at the start of words. With no words it is
// `fallback` when there is one, otherwise an error: the wrapper needs a command.
func launchedFrom(words []*syntax.Word, fallback string) ([]shellCommand, error) {
	if len(words) == 0 {
		if fallback != "" {
			return []shellCommand{{name: fallback}}, nil
		}
		return nil, fmt.Errorf("it launches no command")
	}
	name, literal := literalWord(words[0])
	if !literal {
		return nil, fmt.Errorf("the command it launches has no literal name, so it cannot be checked")
	}
	return []shellCommand{{name: name, args: words[1:]}}, nil
}

func isAssignmentWord(word *syntax.Word) bool {
	if len(word.Parts) == 0 {
		return false
	}
	lit, ok := word.Parts[0].(*syntax.Lit)
	return ok && assignmentWord.MatchString(lit.Value)
}

// skipOptions returns the words after the wrapper's own options. It stops at the
// first word that is not an option (a non-literal word is the command), and
// refuses an option it does not know: what that option does cannot be checked.
func skipOptions(args []*syntax.Word, spec optionSpec) ([]*syntax.Word, error) {
	i := 0
	for i < len(args) {
		opt, literal := literalWord(args[i])
		if !literal || !strings.HasPrefix(opt, "-") {
			break
		}
		if opt == "--" {
			i++
			break
		}
		consumed, err := optionWidth(opt, i+1 < len(args), spec)
		if err != nil {
			return nil, err
		}
		i += consumed
	}
	if i += spec.skip; i > len(args) {
		return nil, fmt.Errorf("it launches no command")
	}
	return args[i:], nil
}

// optionWidth is how many words an option takes: 1, or 2 when its value is the next word.
func optionWidth(opt string, hasNext bool, spec optionSpec) (int, error) {
	name, _, attached := strings.Cut(opt, "=")
	switch {
	case slices.Contains(spec.flags, opt), spec.numeric && numericOption.MatchString(opt):
		return 1, nil
	case strings.HasPrefix(opt, "--") && attached && slices.Contains(spec.valueOpts, name):
		return 1, nil
	case slices.Contains(spec.valueOpts, opt):
		if !hasNext {
			return 0, fmt.Errorf("option %s is missing its value", opt)
		}
		return 2, nil
	case !strings.HasPrefix(opt, "--") && len(opt) > 2 && slices.Contains(spec.valueOpts, opt[:2]):
		return 1, nil // -n5, -I{}
	}
	return 0, fmt.Errorf("option %s is not one it can check", opt)
}
