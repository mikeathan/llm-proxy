package nmap

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/platform/progexec"
	"llm-proxy/internal/platform/sandbox"
)

type fakeRunner struct {
	out    progexec.Output
	err    error
	gotInv progexec.Invocation
	rootOK bool
}

func (f *fakeRunner) Run(_ context.Context, _ progexec.Program, inv progexec.Invocation) (progexec.Output, error) {
	f.gotInv = inv
	if st, err := os.Stat(inv.Root); err == nil && st.IsDir() {
		f.rootOK = true
	}
	return f.out, f.err
}

func testEngine(t *testing.T, runner Runner) (*Engine, string) {
	t.Helper()
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, nmapDataFile), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := New(runner)
	e.dataDirs = []string{filepath.Join(dataDir, "missing"), dataDir}
	e.detect = func(context.Context) (progexec.Installed, error) {
		return progexec.Installed{Path: "/usr/bin/nmap", Version: "Nmap 7.94"}, nil
	}
	return e, dataDir
}

func TestEngine_ScanBuildsAGuardedInvocationAndParsesTheXML(t *testing.T) {
	xml, err := os.ReadFile("testdata/services_localhost.xml")
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{out: progexec.Output{Stdout: xml}}
	e, dataDir := testEngine(t, runner)

	res, err := e.Scan(context.Background(), netaudit.ScanRequest{
		Targets: []netip.Addr{netip.MustParseAddr("192.168.1.5")}, Mode: netaudit.ModeServices, Ports: "80",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) != 1 || len(res.Hosts[0].Ports) != 2 {
		t.Errorf("hosts = %+v", res.Hosts)
	}
	inv := runner.gotInv
	if !slices.Contains(inv.Env, "NMAPDIR="+dataDir) {
		t.Errorf("nmap must be pinned to its data dir, env = %v", inv.Env)
	}
	for _, kv := range inv.Env {
		if strings.HasPrefix(kv, "HOME=") && kv != "HOME="+inv.Root {
			t.Errorf("HOME must be the private run dir: %v", inv.Env)
		}
	}
	if !slices.Contains(inv.ReadPaths, dataDir) || !slices.Contains(inv.ReadPaths, "/usr/bin") {
		t.Errorf("read paths = %v", inv.ReadPaths)
	}
	if !runner.rootOK {
		t.Error("the run directory must exist while nmap runs")
	}
	if _, statErr := os.Stat(inv.Root); !os.IsNotExist(statErr) {
		t.Error("the run directory must be removed after the scan")
	}
}

func TestEngine_ScanSurfacesRunnerErrorsAndTruncation(t *testing.T) {
	boom := errors.New("boom")
	e, _ := testEngine(t, &fakeRunner{err: boom})
	req := netaudit.ScanRequest{Targets: []netip.Addr{netip.MustParseAddr("10.0.0.1")}, Mode: netaudit.ModeDiscover}
	if _, err := e.Scan(context.Background(), req); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}

	e, _ = testEngine(t, &fakeRunner{out: progexec.Output{Stdout: []byte("<nmaprun></nmaprun>"), Truncated: true}})
	res, err := e.Scan(context.Background(), req)
	if err != nil || len(res.Warnings) != 1 {
		t.Errorf("a truncated output must warn: %+v, %v", res, err)
	}
}

func TestEngine_AvailableReportsWhatIsMissing(t *testing.T) {
	e, _ := testEngine(t, &fakeRunner{})
	if err := e.Available(context.Background()); err != nil {
		t.Errorf("Available = %v", err)
	}

	e.dataDirs = []string{t.TempDir()}
	if err := e.Available(context.Background()); err == nil || !strings.Contains(err.Error(), nmapDataFile) {
		t.Errorf("missing data files must be reported, got %v", err)
	}

	e, _ = testEngine(t, &fakeRunner{})
	e.detect = func(context.Context) (progexec.Installed, error) {
		return progexec.Installed{}, &progexec.NotInstalledError{Program: "nmap", Hint: "brew install nmap"}
	}
	var missing *progexec.NotInstalledError
	if err := e.Available(context.Background()); !errors.As(err, &missing) {
		t.Errorf("err = %v, want NotInstalledError", err)
	}
	if !e.Supports(netaudit.ModeServices) || e.Name() != "nmap" {
		t.Error("nmap supports every mode")
	}
}

// TestMain lets the test binary act as the sandbox runner: under Landlock the engine starts nmap by re-running this
// binary, which confines itself and then execs nmap.
func TestMain(m *testing.M) {
	sandbox.RunChildIfRequested()
	os.Exit(m.Run())
}

// With the real sandbox provider, real nmap must run to completion inside the jail in every mode. This is what caught
// nmap refusing to start because /dev/null was not writable. It needs nmap and an OS file jail, so it skips elsewhere.
func TestEngine_RunsRealNmapInsideTheFileJail(t *testing.T) {
	if _, err := exec.LookPath("nmap"); err != nil {
		t.Skip("nmap is not installed")
	}
	sb := sandbox.New(sandbox.Config{Filesystem: true})
	if sb.Effective().Filesystem.Mechanism == sandbox.EnforcementNone {
		t.Skip("no OS file jail on this host")
	}
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
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	e := New(progexec.Runner{Sandbox: sb}) // no AllowUnconfined: the jail must be in force
	if err := e.Available(context.Background()); err != nil {
		t.Skipf("nmap not usable here: %v", err)
	}
	for _, mode := range netaudit.Modes() {
		res, err := e.Scan(context.Background(), netaudit.ScanRequest{
			Targets: []netip.Addr{netip.MustParseAddr("127.0.0.1")}, Mode: mode, Ports: port, Prefiltered: true,
		})
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if len(res.Hosts) != 1 || !res.Hosts[0].Up {
			t.Errorf("%s: hosts = %+v", mode, res.Hosts)
		}
		if mode != netaudit.ModeDiscover && (len(res.Hosts[0].Ports) != 1 || res.Hosts[0].Ports[0].State != "open") {
			t.Errorf("%s: the open port was not found: %+v", mode, res.Hosts[0].Ports)
		}
	}
}
