package proxy

import "os/exec"

// Allow tests to override exec.Command
var execCommand = exec.Command

func SetExecCommand(cmd func(name string, arg ...string) *exec.Cmd) func() {
	orig := execCommand
	execCommand = cmd
	return func() { execCommand = orig }
}
