package isolation_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"llm-proxy/internal/platform/isolation"
	"llm-proxy/internal/platform/isolation/isolationtest"
)

// TestMain lets the test binary act as both halves of a confined run: the sandbox runner that confines itself and execs the
// program, and the probe program the conformance suite runs inside that confinement.
func TestMain(m *testing.M) {
	isolation.RunChildIfRequested()
	isolationtest.RunProbeIfRequested()
	os.Exit(m.Run())
}

// Every guarantee the library makes is checked in one place, wherever the host can enforce it, and a guarantee the host cannot
// enforce is reported with a reason instead of passing silently.
func TestConformance(t *testing.T) {
	isolationtest.Run(t)
}

// A profile starts with every relaxation off: no Unix sockets, no network opt-out, a clean environment.
func TestProfileFor_StartsWithNoRelaxations(t *testing.T) {
	p := isolation.ProfileFor("/work")
	if p.Root != "/work" || p.NoNetwork || p.AllowUnixSockets || len(p.ReadPaths) != 0 {
		t.Errorf("profile = %+v", p)
	}
	if p.Env == nil || p.Timeout <= 0 || p.MaxOutput <= 0 {
		t.Errorf("a profile must carry an explicit environment, a timeout and an output cap: %+v", p)
	}
	for _, kv := range p.Env {
		if len(kv) >= 5 && kv[:5] == "HOME=" && kv != "HOME=/work" {
			t.Errorf("HOME = %q, want it inside the root", kv)
		}
	}
}

// An incomplete profile is refused before anything starts, so a caller cannot run a program by leaving a guarantee out.
func TestRunner_RefusesAnIncompleteProfile(t *testing.T) {
	r := isolation.NewRunner(isolation.New(isolation.Config{}), func() bool { return true })
	prog := isolation.Program{Name: "true", Binary: "true"}
	for name, mutate := range map[string]func(*isolation.Profile){
		"no environment": func(p *isolation.Profile) { p.Env = nil },
		"no root":        func(p *isolation.Profile) { p.Root = "" },
		"no timeout":     func(p *isolation.Profile) { p.Timeout = 0 },
		"no output cap":  func(p *isolation.Profile) { p.MaxOutput = 0 },
	} {
		p := isolation.ProfileFor(t.TempDir())
		mutate(&p)
		if _, err := r.Run(context.Background(), prog, p); !errors.Is(err, isolation.ErrBadInvocation) {
			t.Errorf("%s: err = %v, want ErrBadInvocation", name, err)
		}
	}
}

// With no OS confinement and no operator opt-in, nothing starts.
func TestRunner_DoesNotRunUnconfinedUnlessTheOperatorAllows(t *testing.T) {
	r := isolation.NewRunner(isolation.New(isolation.Config{}), nil) // filesystem confinement not requested: the no-op provider
	p := isolation.ProfileFor(t.TempDir())
	p.Timeout = 5 * time.Second
	if _, err := r.Run(context.Background(), isolation.Program{Name: "true", Binary: "true"}, p); !errors.Is(err, isolation.ErrUnconfined) {
		t.Errorf("err = %v, want ErrUnconfined", err)
	}
}
