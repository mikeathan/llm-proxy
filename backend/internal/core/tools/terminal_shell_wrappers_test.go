package tools

import (
	"testing"

	"llm-proxy/models"
)

// A wrapper (env, xargs, find -exec, nohup, timeout …) runs another program by
// argument. Allowing the wrapper must not allow whatever it launches: the
// launched command is judged like any other, however deeply wrapped.

var wrapperCfg = models.TerminalGuardrailsConfig{
	Enabled:         true,
	AllowedCommands: []string{"ls", "echo", "cat", "env", "find", "xargs", "nohup", "nice", "timeout", "command", "exec", "sudo", "bash", "sh", "builtin", "eval", "grep"},
}

func TestValidateTerminalCommand_WrappedCommandsAreChecked(t *testing.T) {
	refused := []string{
		"env rm y",
		"env FOO=1 BAR=2 rm y",
		"env -i rm y",
		"env -u HOME rm y",
		"env -- rm y",
		"env /bin/rm y",
		"/usr/bin/env rm y",
		"nohup rm y",
		"nice rm y",
		"nice -n 5 rm y",
		"nice -5 rm y",
		"timeout 5 rm y",
		"timeout -s KILL 5 rm y",
		"timeout --signal=KILL -k 2 5 rm y",
		"command rm y",
		"command -p rm y",
		"builtin rm y",
		"exec rm y",
		"sudo rm y",
		"sudo -u root rm y",
		"xargs rm",
		"xargs -0 -I{} rm {}",
		"xargs -n 1 rm",
		"ls | xargs -r rm",
		"find . -exec rm {} \\;",
		"find . -name x -execdir rm {} +",
		"find . -ok rm {} \\;",
		"find . -exec ls {} \\; -exec rm {} \\;",
		"env nohup rm y",
		"sudo env FOO=1 timeout 5 xargs rm",
		"find . -exec env rm {} \\;",
		"find . -exec bash -c 'rm y' \\;",
		"env bash -c 'rm y'",
		"env /bin/bash -c 'rm y'",
		"nohup sh -lc 'rm y'",
		"eval rm y",
		"eval 'ls; rm y'",
		"eval ls $X",
		"echo x | xargs sh -c 'rm y'",
	}
	for _, c := range refused {
		if err := ValidateTerminalCommand(c, wrapperCfg, nil, nil, ""); err == nil {
			t.Errorf("expected %q to be refused", c)
		}
	}
}

func TestValidateTerminalCommand_WrappedAllowedCommandsPass(t *testing.T) {
	allowed := []string{
		"env",
		"env FOO=1 ls",
		"env -i FOO=1 ls -la",
		"env -u HOME ls",
		"nohup ls",
		"nice -n 5 ls",
		"timeout 5 ls",
		"timeout $T ls",
		"timeout -s KILL 5 ls",
		"command ls",
		"command -v rm",
		"command -V rm",
		"exec ls",
		"sudo -u root ls",
		"xargs",
		"ls | xargs echo",
		"ls | xargs -n 1 -I{} echo {}",
		"ls | xargs -0 grep x",
		"find . -name '*.md'",
		"find . -exec ls {} \\;",
		"find . -name x -execdir cat {} +",
		"find $DIR -name x",
		"env nohup ls",
		"find . -exec env ls {} \\;",
		"env bash -c 'ls'",
		"eval ls",
		"eval 'ls | grep x'",
	}
	for _, c := range allowed {
		if err := ValidateTerminalCommand(c, wrapperCfg, nil, nil, ""); err != nil {
			t.Errorf("expected %q to validate, got %v", c, err)
		}
	}
}

func TestValidateTerminalCommand_WrapperThatCannotBeReadIsRefused(t *testing.T) {
	refused := []string{
		"env -S 'rm y'",
		"env --split-string='rm y'",
		"env --made-up ls",
		"env $X",
		"env FOO=1 $X",
		"xargs --made-up ls",
		"xargs $X",
		"nohup $X",
		"sudo -i",
		"sudo -s",
		"sudo --made-up ls",
		"find . -exec $X {} \\;",
		"find . -exec",
		"builtin $X",
		"nice -n",
		"timeout",
	}
	for _, c := range refused {
		if err := ValidateTerminalCommand(c, wrapperCfg, nil, nil, ""); err == nil {
			t.Errorf("expected %q to be refused", c)
		}
	}
}

func TestValidateTerminalCommand_ShellReadingStdinIsRefused(t *testing.T) {
	refused := []string{
		"echo rm y | bash",
		"echo rm y | sh",
		"bash",
		"bash -s",
		"bash -",
		"sh -s -- arg",
		"bash <<< 'rm y'",
		"cat f | /bin/bash",
		"env bash",
		"xargs bash",
		"bash -i",
		"bash <(echo 'rm y')",
		"echo 'rm y' | bash /dev/stdin",
		"echo 'rm y' | sh /dev/fd/0",
		"echo 'rm y' | bash /proc/self/fd/0",
		"echo 'rm y' | bash +x",
		"echo 'rm y' | bash +s",
		"bash $SCRIPT",
		"bash +o posix",
	}
	for _, c := range refused {
		if err := ValidateTerminalCommand(c, wrapperCfg, nil, nil, ""); err == nil {
			t.Errorf("expected %q to be refused", c)
		}
	}
	for _, c := range []string{"sh smoke-test-dir/hello.txt", "bash script.sh arg", "bash +x script.sh", "bash --version", "bash -c 'ls'", "bash -lc 'ls'"} {
		if err := ValidateTerminalCommand(c, wrapperCfg, nil, nil, ""); err != nil {
			t.Errorf("expected %q to validate, got %v", c, err)
		}
	}
}

func TestShellCommandNames_IncludesWhatWrappersLaunch(t *testing.T) {
	cases := map[string][]string{
		"env FOO=1 rm y":          {"env", "rm"},
		"find . -exec rm {} \\;":  {"find", "rm"},
		"ls | xargs rm":           {"ls", "xargs", "rm"},
		"sudo env timeout 5 rm y": {"sudo", "env", "timeout", "rm"},
		"xargs":                   {"xargs", "echo"},
	}
	for command, want := range cases {
		got, err := ShellCommandNames(command)
		if err != nil {
			t.Errorf("%q: %v", command, err)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%q: got %v, want %v", command, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%q: got %v, want %v", command, got, want)
				break
			}
		}
	}
}
