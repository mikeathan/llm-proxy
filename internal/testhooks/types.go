package testhooks

import (
	"llm-proxy/utils"
	"os/exec"
)

var PortReady = utils.PortReady
var ExecCommand = exec.Command

// Allow tests to override.
func SetPortReady(fn func(int) bool) func() {
	orig := PortReady
	PortReady = fn
	return func() { PortReady = orig }
}

// Allow tests to override exec.Command.
func SetExecCommand(cmd func(name string, arg ...string) *exec.Cmd) func() {
	orig := ExecCommand
	ExecCommand = cmd
	return func() { ExecCommand = orig }
}
