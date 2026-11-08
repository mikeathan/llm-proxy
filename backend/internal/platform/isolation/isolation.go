// Package isolation is the one door to the host's program isolation: the OS sandbox (platform/sandbox) and the guarded-program
// runner (platform/progexec) behind a single Profile, so a caller states what a run is allowed in one value and the conformance
// suite (isolationtest) checks every guarantee in one place.
//
// It adds no behaviour. Every type below is the underlying package's own, and a Run is exactly a progexec run with the
// invocation built from the Profile. Isolation is core, not a stack (ADR-0033): it cannot be swapped or switched off by anything
// a stack declares, and a Profile starts with every relaxation off.
package isolation

import (
	"context"
	"time"

	"llm-proxy/internal/platform/progexec"
	"llm-proxy/internal/platform/sandbox"
)

// The sandbox and runner types, re-exported so a caller imports one package.
type (
	Provider       = sandbox.Provider
	Config         = sandbox.Config
	EffectiveState = sandbox.EffectiveState
	SurfaceState   = sandbox.SurfaceState
	Enforcement    = sandbox.Enforcement
	WorkspaceView  = sandbox.WorkspaceView
	Program        = progexec.Program
	Output         = progexec.Output
	Installed      = progexec.Installed
)

// Enforcement values a surface can report.
const (
	EnforcementNone     = sandbox.EnforcementNone
	EnforcementLandlock = sandbox.EnforcementLandlock
	EnforcementSeccomp  = sandbox.EnforcementSeccomp
)

// Typed refusals; match them with errors.Is or errors.As.
var (
	ErrUnconfined    = progexec.ErrUnconfined
	ErrBadInvocation = progexec.ErrBadInvocation
	ErrTimedOut      = progexec.ErrTimedOut
)

// NotInstalledError reports a missing program together with how to install it.
type NotInstalledError = progexec.NotInstalledError

// New selects the host's provider from the operator's request (see sandbox.New). A surface the host cannot confine is reported by
// Effective, never claimed.
func New(cfg Config) Provider { return sandbox.New(cfg) }

// RunChildIfRequested makes the process act as the sandbox runner when it was started as one. Call it first in main and in any
// test binary that runs confined programs.
func RunChildIfRequested() bool { return sandbox.RunChildIfRequested() }

// Detect finds a program and asks it for its version.
func Detect(ctx context.Context, p Program) (Installed, error) { return progexec.Detect(ctx, p) }

// Profile is everything one guarded run is allowed. The zero value of a relaxation is "refused".
type Profile struct {
	// Root is the directory the program runs in and may write to; the program is jailed to it.
	Root string
	// ReadPaths are extra directories the program may read, such as its own data files.
	ReadPaths []string
	// NoNetwork denies new TCP binds and connects where the kernel can express it.
	NoNetwork bool
	// AllowUnixSockets is a relaxation: the program may make Unix sockets (a local daemon over D-Bus). Refused by default.
	AllowUnixSockets bool
	// Env is the program's whole environment; nil is refused, because it would inherit the host's.
	Env []string
	// Timeout kills the program and its process group when exceeded.
	Timeout time.Duration
	// MaxOutput caps the stdout bytes kept.
	MaxOutput int
}

// Defaults for ProfileFor.
const (
	defaultTimeout   = time.Minute
	defaultMaxOutput = 1 << 20
)

// ProfileFor is the starting profile for a run rooted at root: the clean environment, a one-minute timeout, a 1 MiB output cap and
// every relaxation off. Callers change only what their program needs.
func ProfileFor(root string) Profile {
	return Profile{Root: root, Env: progexec.CleanEnv(root), Timeout: defaultTimeout, MaxOutput: defaultMaxOutput}
}

func (p Profile) invocation(args []string) progexec.Invocation {
	return progexec.Invocation{
		Args: args, Env: p.Env, Root: p.Root, ReadPaths: p.ReadPaths, NoNetwork: p.NoNetwork,
		AllowUnixSockets: p.AllowUnixSockets, Timeout: p.Timeout, MaxOutput: p.MaxOutput,
	}
}

// Runner starts programs under a Profile through one provider.
type Runner struct {
	inner progexec.Runner
}

// NewRunner returns a Runner over provider. allowUnconfined reports whether the operator lets programs run on a host with no OS file
// confinement; nil means never.
func NewRunner(provider Provider, allowUnconfined func() bool) Runner {
	return Runner{inner: progexec.Runner{Sandbox: provider, AllowUnconfined: allowUnconfined}}
}

// Effective reports what the host actually enforces.
func (r Runner) Effective() EffectiveState { return r.inner.Sandbox.Effective() }

// Run starts the program with args under profile and returns its stdout. It refuses before starting anything when the profile is
// incomplete, or when the host cannot confine the program and the operator has not allowed that.
func (r Runner) Run(ctx context.Context, p Program, profile Profile, args ...string) (Output, error) {
	return r.inner.Run(ctx, p, profile.invocation(args))
}
