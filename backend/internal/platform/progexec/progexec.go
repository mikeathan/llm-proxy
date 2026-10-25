// Package progexec runs a known external program safely on behalf of an agent tool. It knows nothing about what
// the program does: the caller supplies code-built arguments, an explicit environment and a root directory, and
// the runner starts the program through the sandbox spawn path with a timeout and an output cap.
//
// A guarded program is never started with a model-chosen flag, never inherits the host's environment, and — on a
// host with no OS file confinement — is not started at all unless the operator has opted in.
package progexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"llm-proxy/internal/platform/process"
	"llm-proxy/internal/platform/sandbox"
)

// versionTimeout bounds the version query Detect runs.
const versionTimeout = 5 * time.Second

// versionOutputCap bounds how much of the version output Detect reads.
const versionOutputCap = 4096

// killGrace is how long a cancelled program gets to exit after its process group is killed before Wait gives up.
const killGrace = 2 * time.Second

// Typed refusals; match them with errors.Is or errors.As.
var (
	// ErrUnconfined means the host has no OS file confinement and the operator has not allowed running without it.
	ErrUnconfined = errors.New("no OS file confinement on this host; guarded programs do not run unconfined unless the operator allows it")
	// ErrBadInvocation means the caller left out something every guarded run must state.
	ErrBadInvocation = errors.New("invalid guarded-program invocation")
	// ErrTimedOut means the program ran past its timeout and was killed.
	ErrTimedOut = errors.New("program timed out")
)

// Program names a known external program.
type Program struct {
	// Name is how the program is called in messages (for example "nmap").
	Name string
	// Binary is the executable name to look up on PATH, or an absolute path.
	Binary string
	// VersionArgs are the fixed arguments that make the program print its version.
	VersionArgs []string
	// InstallHint maps a GOOS value to the command that installs the program there.
	InstallHint map[string]string
}

// NotInstalledError reports a missing program together with how to install it on this OS.
type NotInstalledError struct {
	Program string
	Hint    string
}

func (e *NotInstalledError) Error() string {
	if e.Hint == "" {
		return fmt.Sprintf("%s is not installed", e.Program)
	}
	return fmt.Sprintf("%s is not installed; install it with: %s", e.Program, e.Hint)
}

// Installed describes a program found on this host.
type Installed struct {
	Path    string
	Version string
}

// Detect finds the program and asks it for its version. A missing program is a *NotInstalledError carrying the
// install command for this OS, the single source of that message. The version query runs with an empty
// environment, a short timeout and a capped read.
func Detect(ctx context.Context, p Program) (Installed, error) {
	path, err := lookPath(p)
	if err != nil {
		return Installed{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := process.AgentCommand(ctx, path, p.VersionArgs...)
	cmd.Env = []string{}
	out := &cappedBuffer{limit: versionOutputCap}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return Installed{}, fmt.Errorf("%s is installed at %s but did not report a version: %w", p.Name, path, err)
	}
	return Installed{Path: path, Version: firstLine(out.Bytes())}, nil
}

// Invocation is one run of a program. Every field but ReadPaths is required.
type Invocation struct {
	// Args are the code-built arguments.
	Args []string
	// Env is the program's whole environment. It must be non-nil: a nil environment would inherit the host's.
	Env []string
	// Root is the directory the program runs in and may write to; the runner confines the program to it.
	Root string
	// ReadPaths are extra directories the program may read, such as its own data files.
	ReadPaths []string
	// Timeout kills the program (and its process group) when exceeded.
	Timeout time.Duration
	// MaxOutput caps the stdout bytes kept; the program keeps running past the cap, its output is dropped.
	MaxOutput int
}

func (i Invocation) validate() error {
	switch {
	case i.Env == nil:
		return fmt.Errorf("%w: Env must be explicit (nil would inherit the host environment)", ErrBadInvocation)
	case i.Root == "":
		return fmt.Errorf("%w: Root is required", ErrBadInvocation)
	case i.Timeout <= 0:
		return fmt.Errorf("%w: Timeout must be positive", ErrBadInvocation)
	case i.MaxOutput <= 0:
		return fmt.Errorf("%w: MaxOutput must be positive", ErrBadInvocation)
	}
	return nil
}

// Output is what a run produced.
type Output struct {
	Stdout    []byte
	Truncated bool // stdout went past MaxOutput and the rest was dropped
}

// Runner starts guarded programs through a sandbox provider.
type Runner struct {
	Sandbox sandbox.Provider
	// AllowUnconfined reports whether the operator allows running on a host with no OS file confinement. Nil means
	// never.
	AllowUnconfined func() bool
}

// Run starts the program and returns its stdout. On a timeout it returns ErrTimedOut together with whatever stdout there was. It refuses before starting anything when the invocation is
// incomplete or the host cannot confine the program and the operator has not allowed that. The program is killed
// with its whole process group when ctx ends or the timeout passes.
func (r Runner) Run(ctx context.Context, p Program, inv Invocation) (Output, error) {
	if err := inv.validate(); err != nil {
		return Output{}, err
	}
	if err := r.checkConfinement(); err != nil {
		return Output{}, err
	}
	installed, err := lookPath(p)
	if err != nil {
		return Output{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, inv.Timeout)
	defer cancel()
	cmd := process.AgentCommand(ctx, installed, inv.Args...)
	cmd.Env = append([]string{}, inv.Env...)
	cmd.Dir = inv.Root
	cmd.Cancel = func() error { return process.KillGroup(cmd, syscall.SIGKILL) }
	cmd.WaitDelay = killGrace
	stdout := &cappedBuffer{limit: inv.MaxOutput}
	stderr := &cappedBuffer{limit: versionOutputCap}
	cmd.Stdout, cmd.Stderr = stdout, stderr

	if err := r.Sandbox.Wrap(cmd, sandbox.WorkspaceView{RootPath: inv.Root, NetworkOn: true, ReadPaths: inv.ReadPaths}); err != nil {
		return Output{}, fmt.Errorf("failed to confine %s: %w", p.Name, err)
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// What the program printed before it was killed is returned with the error: a long scan that finished some
			// hosts has results worth keeping.
			return Output{Stdout: stdout.Bytes(), Truncated: stdout.truncated}, fmt.Errorf("%s: %w after %s", p.Name, ErrTimedOut, inv.Timeout)
		}
		if ctx.Err() != nil {
			return Output{}, ctx.Err()
		}
		return Output{}, fmt.Errorf("%s failed: %w: %s", p.Name, err, firstLine(stderr.Bytes()))
	}
	return Output{Stdout: stdout.Bytes(), Truncated: stdout.truncated}, nil
}

// checkConfinement refuses to run on a host with no OS file confinement unless the operator opted in. It reads
// what the provider actually enforces, not the OS name.
func (r Runner) checkConfinement() error {
	if r.Sandbox == nil {
		return fmt.Errorf("%w: no sandbox provider", ErrUnconfined)
	}
	if r.Sandbox.Effective().Filesystem.Mechanism != sandbox.EnforcementNone {
		return nil
	}
	if r.AllowUnconfined != nil && r.AllowUnconfined() {
		return nil
	}
	return ErrUnconfined
}

func lookPath(p Program) (string, error) {
	path, err := exec.LookPath(p.Binary)
	if err != nil {
		return "", &NotInstalledError{Program: p.Name, Hint: p.InstallHint[runtime.GOOS]}
	}
	return path, nil
}

// CleanEnv is the environment a guarded program normally runs with: a fixed PATH, HOME and TMPDIR inside root, and
// the C locale. Nothing of the host's environment is copied.
func CleanEnv(root string) []string {
	return []string{"PATH=/usr/bin:/bin", "HOME=" + root, "TMPDIR=" + root, "LC_ALL=C"}
}

// cappedBuffer keeps the first limit bytes written to it and drops the rest, still accepting every write so the
// program is never blocked on a full pipe.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	room := c.limit - c.buf.Len()
	if room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	if len(p) > room {
		c.truncated = true
	}
	return len(p), nil
}

func (c *cappedBuffer) Bytes() []byte { return c.buf.Bytes() }

func firstLine(b []byte) string {
	line, _, _ := bytes.Cut(bytes.TrimSpace(b), []byte("\n"))
	return string(line)
}
