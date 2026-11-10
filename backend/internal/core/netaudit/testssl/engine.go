package testssl

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"time"

	"llm-proxy/internal/platform/isolation"
)

// Run limits. A look makes a few hundred connections to one service, so it gets longer than a port scan of one device.
const (
	lookTimeout   = 3 * time.Minute
	maxOutputFile = 1 << 20
	runDirPattern = "netaudit-testssl-*"
	resultFile    = "testssl.json"
)

// Program is how testssl.sh is found and installed. It is the single source of the install message.
var Program = isolation.Program{
	Name:        "testssl",
	Binary:      "testssl.sh",
	Alternates:  []string{"testssl"}, // Debian and Ubuntu install the package's program under this name
	VersionArgs: []string{"--version"},
	InstallHint: map[string]string{
		"darwin": "brew install testssl",
		"linux":  "sudo apt install testssl.sh   (or download it from https://testssl.sh)",
	},
}

// Present says whether testssl.sh is installed, without starting it: nil, or a *isolation.NotInstalledError with the install command for this OS.
func Present() error { return isolation.Present(Program) }

// ErrOutOfScope means the address is not one the declared scan scope allows, so nothing was run.
var ErrOutOfScope = errors.New("testssl: the address is outside the declared scan scope")

// Runner starts guarded programs; isolation.Runner satisfies it.
type Runner interface {
	Run(ctx context.Context, p isolation.Program, profile isolation.Profile, args ...string) (isolation.Output, error)
}

// Scope authorises addresses against the declared scan scope; netaudit.PolicyAuthorizer satisfies it. The program opens its own sockets, so
// the scoped dial cannot cover it (ADR-0039): the address is checked here, before the program is started.
type Scope interface {
	Authorize(ctx context.Context, targets []string) ([]netip.Addr, error)
}

// Engine runs testssl.sh through a guarded-program runner.
type Engine struct {
	runner Runner
	scope  Scope
	detect func(context.Context) (isolation.Installed, error)
}

// New builds the engine. Detection is not cached, so installing the program takes effect on the next look.
func New(runner Runner, scope Scope) *Engine {
	return &Engine{
		runner: runner, scope: scope,
		detect: func(ctx context.Context) (isolation.Installed, error) { return isolation.Detect(ctx, Program) },
	}
}

// Available reports why the program cannot be used; nil means it is installed.
func (e *Engine) Available(ctx context.Context) error {
	_, err := e.detect(ctx)
	return err
}

// Look runs the program against one service and returns the typed result. An address the scope does not allow is refused before anything starts.
// A run that was cut short, failed or wrote nothing readable returns an error and no verdicts: a service that could not be tested is unknown,
// never clean.
func (e *Engine) Look(ctx context.Context, t Target) (Result, error) {
	if err := e.authorize(ctx, t); err != nil {
		return Result{}, err
	}
	installed, err := e.detect(ctx)
	if err != nil {
		return Result{}, err
	}
	// The scratch folder is private to the look and outside every agent workspace; the program's HOME, temp files and result go there.
	root, err := os.MkdirTemp("", runDirPattern)
	if err != nil {
		return Result{}, fmt.Errorf("failed to create the look's directory: %w", err)
	}
	defer os.RemoveAll(root)
	file := filepath.Join(root, resultFile)
	args, err := Args(t, file)
	if err != nil {
		return Result{}, err
	}
	profile := isolation.ProfileFor(root)
	profile.ReadPaths = readPathsFor(installed.Path)
	profile.Timeout = lookTimeout
	profile.MaxOutput = maxOutputFile
	if _, err := e.runner.Run(ctx, Program, profile, args...); err != nil {
		return Result{}, err
	}
	data, err := readCapped(file)
	if err != nil {
		return Result{}, err
	}
	return Parse(data)
}

// readPathsFor names what the jail must let the program read: the folder it was found in and, when that is a link, the folder the link points to
// (Debian links /usr/bin/testssl to a script in /usr/share/testssl.sh that reads its own data files there). Folders are resolved so the grant
// names the real place.
func readPathsFor(program string) []string {
	var out []string
	add := func(dir string) {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			dir = r
		}
		if !slices.Contains(out, dir) {
			out = append(out, dir)
		}
	}
	add(filepath.Dir(program))
	if real, err := filepath.EvalSymlinks(program); err == nil {
		add(filepath.Dir(real))
	}
	return out
}

// authorize asks the scope about this one address and requires the answer to be that address.
func (e *Engine) authorize(ctx context.Context, t Target) error {
	allowed, err := e.scope.Authorize(ctx, []string{t.Addr.String()})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrOutOfScope, err)
	}
	if len(allowed) != 1 || allowed[0] != t.Addr {
		return ErrOutOfScope
	}
	return nil
}

// readCapped reads the result file, refusing one larger than the bound.
func readCapped(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: the program wrote no result", ErrUnreadable)
	}
	if st.Size() > maxOutputFile {
		return nil, fmt.Errorf("%w: the result file is larger than %d bytes", ErrUnreadable, maxOutputFile)
	}
	return os.ReadFile(path)
}
