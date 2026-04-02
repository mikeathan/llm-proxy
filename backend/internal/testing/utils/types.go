package utils

import (
	"context"
	"llm-proxy/utils"
	"os/exec"
)

var PortReady = utils.PortReady
var ExecCommand = exec.Command
var ExecCommandContext = exec.CommandContext

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

func SetExecCommandContext(cmd func(ctx context.Context, name string, arg ...string) *exec.Cmd) func() {
	orig := ExecCommandContext
	ExecCommandContext = cmd
	return func() { ExecCommandContext = orig }
}
