package proxy

import (
	"llm-proxy/utils"
	"os/exec"
)

var portReadyFunc = utils.PortReady

// Allow tests to override
func SetPortReady(fn func(int) bool) func() {
	orig := portReadyFunc
	portReadyFunc = fn
	return func() { portReadyFunc = orig }
}

// Allow tests to override exec.Command
var execCommand = exec.Command

func SetExecCommand(cmd func(name string, arg ...string) *exec.Cmd) func() {
	orig := execCommand
	execCommand = cmd
	return func() { execCommand = orig }
}
