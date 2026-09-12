// Package process centralizes how agent child processes are created and
// reaped (agent-os-sandboxing plan R1 / Phase 2). Every agent-triggered child —
// the persistent shell factory and executeLocal one-shots — is created here so
// process-group isolation is in exactly one place; the sandbox Wrap attaches at
// the two spawn sites through the shared platform/sandbox seam (pooled shell +
// executeLocal), which these helpers hand the same *exec.Cmd to.
package process

import (
	"context"
	"os/exec"
	"syscall"
)

// AgentCommand creates the exec.Cmd for an agent child with standard isolation:
// its own process group (Setpgid), so kill-group semantics terminate the child
// AND any descendants; and context-bound via exec.CommandContext (cancellation
// kills the command — group kill is the caller's job via KillGroup).
func AgentCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// KillGroup terminates the whole process group (negative pid). It is a no-op
// when the command was never started. Safe to call repeatedly.
func KillGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, sig)
}
