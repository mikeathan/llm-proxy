package tools

import (
	"testing"

	"llm-proxy/models"
)

// The whitelist must judge the commands a shell would run, not the words of a
// text split: syntax (for/do/done/if/then/fi, assignments, subshells) is not a
// command, and a command is a command wherever it sits (loop body, background
// job, command substitution, backticks).

var shellSyntaxCfg = models.TerminalGuardrailsConfig{
	Enabled:         true,
	AllowedCommands: []string{"ls", "echo", "cat", "wc", "test", "read", "cd", "grep", "bash", "sh"},
}

func TestValidateTerminalCommand_ShellSyntaxIsNotACommand(t *testing.T) {
	allowed := []string{
		"for f in *.md; do wc -l $f; done",
		"for f in a b; do\n  echo $f\ndone",
		"if test -f a; then cat a; else echo none; fi",
		"while read l; do echo $l; done < notes.txt",
		"FOO=1 ls",
		"FOO=1",
		"(cd sub && ls)",
		"{ ls; echo done; }",
		"ls | grep x | wc -l",
		"# a note\nls",
		"cat <<EOF\nfor x in y; do rm z; done\nEOF",
		"cat <<'EOF'\n$(rm y)\nEOF",
		`bash -lc 'ls'`,
		`echo "for do done"`,
	}
	for _, c := range allowed {
		if err := ValidateTerminalCommand(c, shellSyntaxCfg, nil, nil, ""); err != nil {
			t.Errorf("expected %q to validate, got %v", c, err)
		}
	}
}

func TestValidateTerminalCommand_CommandsAreCheckedWhereverTheySit(t *testing.T) {
	refused := []string{
		"for x in 1; do rm y; done",
		"if test -f a; then rm a; fi",
		"while true; do rm y; done",
		"ls & rm y",
		"ls &\nrm y",
		"echo $(rm y)",
		"echo `rm y`",
		`echo "$(rm y)"`,
		"cat <(rm y)",
		"(rm y)",
		"{ rm y; }",
		"f() { rm y; }; f",
		"FOO=1 rm y",
		"ls && (cd x; rm y)",
		"time rm y",
		"! rm y",
		`bash -c 'for x in 1; do rm y; done'`,
		`bash -c "$(rm y)"`,
		`bash -lc 'rm y'`,
		`sh -ec "rm y"`,
		"cat <<EOF\n$(rm y)\nEOF",
		"cat <<EOF\n`rm y`\nEOF",
	}
	for _, c := range refused {
		if err := ValidateTerminalCommand(c, shellSyntaxCfg, nil, nil, ""); err == nil {
			t.Errorf("expected %q to be refused", c)
		}
	}
}

func TestValidateTerminalCommand_FailsClosedWhenTheNameIsNotLiteral(t *testing.T) {
	refused := []string{
		"$CMD y",
		"${CMD} y",
		"$(echo rm) y",
		"ls; $CMD",
		"for x in 1; do $x; done",
		`bash -c "$SCRIPT"`,
		"echo (",
		"for x in",
		"fi",
	}
	for _, c := range refused {
		if err := ValidateTerminalCommand(c, shellSyntaxCfg, nil, nil, ""); err == nil {
			t.Errorf("expected %q to be refused", c)
		}
	}
}

// With no allowed-command list there is nothing to verify names against, so
// what cannot be read (a name known only at run time, a shell fed on stdin) is
// not refused for that reason. Commands the shell itself would reject still are.
func TestValidateTerminalCommand_UnreadableNamesPassWithoutAWhitelist(t *testing.T) {
	open := models.TerminalGuardrailsConfig{Enabled: true}
	for _, c := range []string{
		"$CMD y",
		"for t in a b; do $t --version; done",
		"echo hi | bash",
		"env $X",
		"bash $SCRIPT",
		"find . -exec $X {} \\;",
	} {
		if err := ValidateTerminalCommand(c, open, nil, nil, ""); err != nil {
			t.Errorf("expected %q to validate without a whitelist, got %v", c, err)
		}
	}
	for _, c := range []string{"echo (", "fi", "echo 'oops"} {
		if err := ValidateTerminalCommand(c, open, nil, nil, ""); err == nil {
			t.Errorf("expected %q to be refused: the shell would reject it", c)
		}
	}
	// A literal -c program is still validated (blocked patterns apply to it).
	blocking := models.TerminalGuardrailsConfig{Enabled: true, BlockedPatterns: []string{"rm -rf"}}
	if err := ValidateTerminalCommand("bash -c 'rm -rf x'", blocking, nil, nil, ""); err == nil {
		t.Error("expected the inner program to meet the blocked patterns")
	}
}
