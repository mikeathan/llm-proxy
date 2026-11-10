package testssl

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"

	"llm-proxy/internal/platform/isolation"
)

// fakeRunner stands in for the guarded runner: it records the call and writes the JSON file the real program would write.
type fakeRunner struct {
	calls   int
	profile isolation.Profile
	args    []string
	json    string
	err     error
}

func (f *fakeRunner) Run(_ context.Context, _ isolation.Program, profile isolation.Profile, args ...string) (isolation.Output, error) {
	f.calls++
	f.profile, f.args = profile, args
	if f.err != nil {
		return isolation.Output{}, f.err
	}
	if f.json != "" {
		i := slices.Index(args, "--jsonfile")
		if err := os.WriteFile(args[i+1], []byte(f.json), 0o600); err != nil {
			return isolation.Output{}, err
		}
	}
	return isolation.Output{}, nil
}

// scope answers like a scan scope: it authorises the addresses it holds and refuses the rest.
type scope struct{ allowed []netip.Addr }

func (s scope) Authorize(_ context.Context, targets []string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, t := range targets {
		a := netip.MustParseAddr(t)
		if !slices.Contains(s.allowed, a) {
			return nil, errors.New("outside the declared scan scope")
		}
		out = append(out, a)
	}
	return out, nil
}

var (
	device = netip.MustParseAddr("192.168.50.10")
	target = Target{Addr: device, Port: 443}
)

func newEngine(r Runner, sc Scope) *Engine {
	e := New(r, sc)
	e.detect = func(context.Context) (isolation.Installed, error) {
		return isolation.Installed{Path: "/opt/testssl/testssl.sh", Version: "3.2"}, nil
	}
	return e
}

func TestLook_RunsTheFixedArgumentsAndReturnsTheTypedResult(t *testing.T) {
	r := &fakeRunner{json: flat}
	res, err := newEngine(r, scope{allowed: []netip.Addr{device}}).Look(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if res.Protocols[TLS10] != Offered || res.Ciphers[Cipher3DES] != Offered {
		t.Errorf("%+v", res)
	}
	if want, _ := Args(target, r.args[slices.Index(r.args, "--jsonfile")+1]); !slices.Equal(r.args, want) {
		t.Errorf("the run used %v, not the fixed list %v", r.args, want)
	}
	if r.profile.Timeout <= 0 || r.profile.MaxOutput <= 0 || r.profile.NoNetwork || r.profile.AllowUnixSockets {
		t.Errorf("a look needs the network, a bound and nothing else: %+v", r.profile)
	}
	if !slices.Contains(r.profile.ReadPaths, "/opt/testssl") {
		t.Errorf("the program's own folder must be readable: %v", r.profile.ReadPaths)
	}
}

// A refused address is never dialled: the runner must not be called at all.
func TestLook_NeverRunsAgainstAnAddressOutsideTheScope(t *testing.T) {
	r := &fakeRunner{json: flat}
	_, err := newEngine(r, scope{allowed: []netip.Addr{netip.MustParseAddr("10.9.9.9")}}).Look(t.Context(), target)
	if !errors.Is(err, ErrOutOfScope) || r.calls != 0 {
		t.Errorf("err %v, %d runs", err, r.calls)
	}
}

// The scope may answer with different addresses than were asked (a name resolved elsewhere); the look is held to the one it was asked for.
func TestLook_RefusesWhenTheScopeAnswersForAnotherAddress(t *testing.T) {
	r := &fakeRunner{json: flat}
	e := newEngine(r, answeringWith{netip.MustParseAddr("10.9.9.9")})
	if _, err := e.Look(t.Context(), target); !errors.Is(err, ErrOutOfScope) || r.calls != 0 {
		t.Errorf("err %v, %d runs", err, r.calls)
	}
}

type answeringWith struct{ addr netip.Addr }

func (a answeringWith) Authorize(context.Context, []string) ([]netip.Addr, error) {
	return []netip.Addr{a.addr}, nil
}

func TestLook_ANotInstalledProgramIsReportedAsThat(t *testing.T) {
	r := &fakeRunner{}
	e := newEngine(r, scope{allowed: []netip.Addr{device}})
	e.detect = func(context.Context) (isolation.Installed, error) {
		return isolation.Installed{}, &isolation.NotInstalledError{Program: "testssl", Hint: "brew install testssl"}
	}
	var missing *isolation.NotInstalledError
	if _, err := e.Look(t.Context(), target); !errors.As(err, &missing) || r.calls != 0 {
		t.Errorf("err %v, %d runs", err, r.calls)
	}
}

// A run that was cut short, failed or wrote nothing readable gives an error and no verdicts: it is "unknown", never "clean".
func TestLook_ARunThatDidNotFinishGivesNoVerdicts(t *testing.T) {
	for name, r := range map[string]*fakeRunner{
		"timed out":     {json: flat, err: isolation.ErrTimedOut},
		"failed":        {err: errors.New("exit status 1")},
		"wrote nothing": {},
		"wrote garbage": {json: "not json"},
		"wrote no ids":  {json: `[{"id":"scanTime","finding":"1"}]`},
	} {
		res, err := newEngine(r, scope{allowed: []netip.Addr{device}}).Look(t.Context(), target)
		if err == nil || len(res.Protocols) != 0 || len(res.Ciphers) != 0 {
			t.Errorf("%s: err %v, result %+v", name, err, res)
		}
	}
}

func TestLook_TheScratchFolderIsGoneAfterwards(t *testing.T) {
	r := &fakeRunner{json: flat}
	if _, err := newEngine(r, scope{allowed: []netip.Addr{device}}).Look(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(r.args[slices.Index(r.args, "--jsonfile")+1])); !os.IsNotExist(err) {
		t.Errorf("the scratch folder was left behind: %v", err)
	}
}

func TestMain(m *testing.M) {
	if isolation.RunChildIfRequested() {
		return
	}
	os.Exit(m.Run())
}

// TestLook_Live runs the real program through the real guarded runner against a service the person names. It is skipped unless
// TESTSSL_LIVE_TARGET is set (host:port of an address that the test scope below allows), so the suite never opens a connection by itself.
func TestLook_Live(t *testing.T) {
	spec := os.Getenv("TESTSSL_LIVE_TARGET")
	if spec == "" {
		t.Skip("set TESTSSL_LIVE_TARGET=ip:port to run the real program")
	}
	ap, err := netip.ParseAddrPort(spec)
	if err != nil {
		t.Fatal(err)
	}
	runner := isolation.NewRunner(isolation.New(isolation.Config{Filesystem: true}), func() bool { return os.Getenv("TESTSSL_LIVE_UNCONFINED") == "1" })
	res, err := New(runner, scope{allowed: []netip.Addr{ap.Addr()}}).Look(t.Context(), Target{Addr: ap.Addr(), Port: ap.Port()})
	if err != nil {
		t.Fatalf("live look: %v", err)
	}
	t.Logf("port %s: %+v", strconv.Itoa(int(ap.Port())), res)
}

func TestPresent_AMissingProgramCarriesTheInstallCommandForThisOS(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var missing *isolation.NotInstalledError
	if err := Present(); !errors.As(err, &missing) || missing.Program != "testssl" {
		t.Fatalf("err %v", err)
	}
	if runtime.GOOS == "darwin" && missing.Hint != "brew install testssl" {
		t.Errorf("hint %q", missing.Hint)
	}
	if missing.Hint == "" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		t.Error("a supported OS must have an install command")
	}
}

func TestPresent_AnInstalledProgramIsNilAndNothingIsStarted(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	script := "#!/bin/sh\ntouch " + marker + "\n"
	if err := os.WriteFile(filepath.Join(dir, "testssl.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := Present(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("looking for the program must not start it")
	}
}

// Debian installs the program as a link in /usr/bin to a script that reads its data files from /usr/share/testssl.sh, and Homebrew links into a
// Cellar. The jail grants only what is named, so both the link's folder and the folder it resolves to are named (seen on Linux 2026-11-10: with
// only /usr/bin the program stalled after its first line and read nothing).
func TestReadPathsFor_NamesTheFolderOfTheLinkAndOfWhereItPoints(t *testing.T) {
	root := t.TempDir()
	share, bin := filepath.Join(root, "share", "testssl.sh"), filepath.Join(root, "bin")
	for _, d := range []string{share, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(share, "testssl.sh")
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bin, "testssl")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	got := readPathsFor(link)
	resolvedShare, _ := filepath.EvalSymlinks(share)
	resolvedBin, _ := filepath.EvalSymlinks(bin)
	if !slices.Contains(got, resolvedShare) || !slices.Contains(got, resolvedBin) || len(got) != 2 {
		t.Errorf("want the link's folder and its target's folder, got %v", got)
	}
	if got := readPathsFor(real); len(got) != 1 {
		t.Errorf("a program that is not a link names one folder: %v", got)
	}
}
