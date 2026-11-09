// Package isolationtest is the conformance suite for the isolation library. It runs one probe program inside the confinement for
// each guarantee and states, per guarantee, whether the host enforced it. A guarantee the host cannot enforce must say why through
// Effective (SPEC-006 II.7); the suite fails if it claims enforcement the probe defeats, or stays silent about a gap.
//
// The test binary that calls Run must start with:
//
//	isolation.RunChildIfRequested()
//	isolationtest.RunProbeIfRequested()
package isolationtest

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"llm-proxy/internal/platform/isolation"
)

// gib is the per-file size limit the suite asks for: one gibibyte (Config.MaxStorageGB = 1).
const gib = 1 << 30

// Run checks every guarantee against the host's real provider.
func Run(t *testing.T) {
	t.Helper()
	s := newSuite(t)
	for _, c := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"filesystem jail", s.filesystem},
		{"tcp denial", s.tcp},
		{"unix socket denial", s.unixSockets},
		{"clean environment", s.environment},
		{"resource limit", s.resourceLimit},
		{"confinement is reported", s.reported},
	} {
		t.Run(c.name, func(t *testing.T) {
			s.t = t // helpers report against the subtest that is running; subtests are not parallel
			c.run(t)
		})
	}
}

type suite struct {
	t       *testing.T
	exe     string
	runner  isolation.Runner
	operUnx isolation.Runner // the operator's sandboxing.unix_sockets switch on
	state   isolation.EffectiveState
}

func newSuite(t *testing.T) *suite {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	allow := func() bool { return true } // so a host with no jail still runs the probe and the suite can read what it reports
	p := isolation.New(isolation.Config{Filesystem: true, MaxStorageGB: 1})
	return &suite{
		t: t, exe: exe, runner: isolation.NewRunner(p, allow), state: p.Effective(),
		operUnx: isolation.NewRunner(isolation.New(isolation.Config{Filesystem: true, AllowUnixSockets: true}), allow),
	}
}

// probe runs one probe action under profile and returns whether it was allowed and the detail it printed.
func (s *suite) probe(r isolation.Runner, p isolation.Profile, action, arg string) (allowed bool, detail string) {
	s.t.Helper()
	p.Env = append(append([]string{}, p.Env...), actionVar+"="+action, argVar+"="+arg)
	out, err := r.Run(context.Background(), isolation.Program{Name: "probe", Binary: s.exe}, p)
	if err != nil {
		s.t.Fatalf("probe %s: %v", action, err)
	}
	line := strings.TrimSpace(string(out.Stdout))
	if v, ok := strings.CutPrefix(line, "OK"); ok {
		return true, strings.TrimSpace(v)
	}
	if strings.HasPrefix(line, "DENIED") {
		return false, line
	}
	s.t.Fatalf("probe %s printed %q", action, line)
	return false, ""
}

func (s *suite) profile() isolation.Profile { return isolation.ProfileFor(s.t.TempDir()) }

// requireEnforced skips a guarantee the host does not enforce, after checking that the host said why.
func (s *suite) requireEnforced(surface isolation.SurfaceState) {
	s.t.Helper()
	if surface.Mechanism != isolation.EnforcementNone {
		return
	}
	if surface.Reason == "" {
		s.t.Fatal("the host enforces nothing here and gives no reason: a gap must be reported, never silent")
	}
	s.t.Skipf("not enforced on this host: %s", surface.Reason)
}

func (s *suite) filesystem(t *testing.T) {
	p := s.profile()
	inside := filepath.Join(p.Root, "in.txt")
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "out.txt")
	for _, f := range []string{inside, outside} {
		if err := os.WriteFile(f, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Positive controls first: a jail that refuses everything would pass the denials below for the wrong reason.
	if ok, d := s.probe(s.runner, p, actionRead, inside); !ok && s.state.Filesystem.Mechanism != isolation.EnforcementNone {
		t.Fatalf("the program cannot read inside its own root: %s", d)
	}
	s.requireEnforced(s.state.Filesystem)
	if ok, _ := s.probe(s.runner, p, actionWrite, filepath.Join(p.Root, "w.txt")); !ok {
		t.Error("the program cannot write inside its own root")
	}
	if ok, _ := s.probe(s.runner, p, actionRead, outside); ok {
		t.Error("the program read a file outside its root")
	}
	if ok, _ := s.probe(s.runner, p, actionWrite, filepath.Join(outsideDir, "w.txt")); ok {
		t.Error("the program wrote a file outside its root")
	}
}

func (s *suite) tcp(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback")
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	addr := ln.Addr().String()
	p := s.profile()
	if ok, d := s.probe(s.runner, p, actionTCP, addr); !ok && s.state.Filesystem.Mechanism != isolation.EnforcementNone {
		t.Fatalf("a program that keeps the network could not connect (a control for the denial below): %s", d)
	}
	s.requireEnforced(s.state.Network)
	p.NoNetwork = true
	if ok, _ := s.probe(s.runner, p, actionTCP, addr); ok {
		t.Error("a no-network program made a TCP connection")
	}
}

func (s *suite) unixSockets(t *testing.T) {
	s.requireEnforced(s.state.UnixSockets)
	p := s.profile()
	if ok, _ := s.probe(s.runner, p, actionUnix, ""); ok {
		t.Error("a default program made a Unix socket")
	}
	p.AllowUnixSockets = true
	if ok, d := s.probe(s.runner, p, actionUnix, ""); !ok {
		t.Errorf("a spawn that allows Unix sockets for itself was refused: %s", d)
	}
	if ok, d := s.probe(s.operUnx, s.profile(), actionUnix, ""); !ok {
		t.Errorf("the operator's switch did not lift the denial: %s", d)
	}
}

func (s *suite) environment(t *testing.T) {
	t.Setenv("ISOTEST_HOST_SECRET", "must-not-reach-the-program")
	_, keys := s.probe(s.runner, s.profile(), actionEnv, "")
	for _, k := range strings.Split(keys, ",") {
		if k == "ISOTEST_HOST_SECRET" || strings.HasPrefix(k, "LLMPROXY_SANDBOX") {
			t.Errorf("the program saw %s; its environment must be exactly the profile's, with no host or runner variables (saw %s)", k, keys)
		}
	}
	if !strings.Contains(keys, "HOME") || !strings.Contains(keys, "PATH") {
		t.Errorf("the clean environment lost HOME or PATH: %s", keys)
	}
}

func (s *suite) resourceLimit(t *testing.T) {
	s.requireEnforced(s.state.Filesystem) // the limit is applied by the confinement runner
	_, got := s.probe(s.runner, s.profile(), actionFSize, "")
	if got != strconv.Itoa(gib) {
		t.Errorf("per-file size limit = %s, want %d", got, gib)
	}
}

// reported checks that every surface says what it does: a mechanism, or a reason for having none.
func (s *suite) reported(t *testing.T) {
	for name, surface := range map[string]isolation.SurfaceState{
		"filesystem": s.state.Filesystem, "network": s.state.Network, "unix_sockets": s.state.UnixSockets,
	} {
		if surface.Mechanism == isolation.EnforcementNone && surface.Reason == "" {
			t.Errorf("%s: no mechanism and no reason", name)
		}
	}
}
