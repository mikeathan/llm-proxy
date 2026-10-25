package progexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/sandbox"
)

// fakeSandbox is a Provider that records what it was asked to wrap and reports a chosen mechanism.
type fakeSandbox struct {
	mechanism sandbox.Enforcement
	wrapped   []sandbox.WorkspaceView
}

func (f *fakeSandbox) Wrap(_ *exec.Cmd, ws sandbox.WorkspaceView) error {
	f.wrapped = append(f.wrapped, ws)
	return nil
}

func (f *fakeSandbox) Effective() sandbox.EffectiveState {
	return sandbox.EffectiveState{Filesystem: sandbox.SurfaceState{Mechanism: f.mechanism}}
}

// script writes an executable shell script and returns the Program for it.
func script(t *testing.T, body string) Program {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fakeprog")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return Program{Name: "fakeprog", Binary: path, VersionArgs: []string{"--version"}}
}

func confined() Runner {
	return Runner{Sandbox: &fakeSandbox{mechanism: sandbox.EnforcementLandlock}}
}

func invocation(t *testing.T, args ...string) Invocation {
	root := t.TempDir()
	return Invocation{Args: args, Env: CleanEnv(root), Root: root, Timeout: 5 * time.Second, MaxOutput: 1024}
}

func TestRun_PassesArgsAndOnlyTheGivenEnvironment(t *testing.T) {
	t.Setenv("PROGEXEC_HOST_SECRET", "leak")
	prog := script(t, `echo "args=$*"; echo "secret=${PROGEXEC_HOST_SECRET:-none}"; echo "home=$HOME"; pwd`)
	inv := invocation(t, "-a", "b")

	out, err := confined().Run(context.Background(), prog, inv)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := string(out.Stdout)
	for _, want := range []string{"args=-a b", "secret=none", "home=" + inv.Root} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if resolved, _ := filepath.EvalSymlinks(inv.Root); !strings.Contains(got, resolved) {
		t.Errorf("the program must run in Root, got:\n%s", got)
	}
}

func TestRun_RefusesAnIncompleteInvocation(t *testing.T) {
	prog := script(t, "echo hi")
	cases := map[string]func(*Invocation){
		"nil env":       func(i *Invocation) { i.Env = nil },
		"no root":       func(i *Invocation) { i.Root = "" },
		"no timeout":    func(i *Invocation) { i.Timeout = 0 },
		"no output cap": func(i *Invocation) { i.MaxOutput = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			inv := invocation(t)
			mutate(&inv)
			if _, err := confined().Run(context.Background(), prog, inv); !errors.Is(err, ErrBadInvocation) {
				t.Errorf("err = %v, want ErrBadInvocation", err)
			}
		})
	}
}

func TestRun_FailsClosedWithoutFilesystemConfinement(t *testing.T) {
	prog := script(t, "echo ran")
	none := &fakeSandbox{mechanism: sandbox.EnforcementNone}

	if _, err := (Runner{Sandbox: none}).Run(context.Background(), prog, invocation(t)); !errors.Is(err, ErrUnconfined) {
		t.Errorf("no confinement and no opt-in must be refused, got %v", err)
	}
	if _, err := (Runner{}).Run(context.Background(), prog, invocation(t)); !errors.Is(err, ErrUnconfined) {
		t.Errorf("no provider must be refused, got %v", err)
	}
	if len(none.wrapped) != 0 {
		t.Error("nothing may be started or wrapped when refused")
	}

	optedIn := Runner{Sandbox: none, AllowUnconfined: func() bool { return true }}
	if out, err := optedIn.Run(context.Background(), prog, invocation(t)); err != nil || !strings.Contains(string(out.Stdout), "ran") {
		t.Errorf("the operator opt-in must allow it: %q, %v", out.Stdout, err)
	}
}

func TestRun_AsksTheSandboxToConfineToRootWithTheReadPaths(t *testing.T) {
	sb := &fakeSandbox{mechanism: sandbox.EnforcementLandlock}
	inv := invocation(t)
	inv.ReadPaths = []string{"/usr/share/nmap"}

	if _, err := (Runner{Sandbox: sb}).Run(context.Background(), script(t, "true"), inv); err != nil {
		t.Fatal(err)
	}
	if len(sb.wrapped) != 1 || sb.wrapped[0].RootPath != inv.Root || sb.wrapped[0].ReadPaths[0] != "/usr/share/nmap" || !sb.wrapped[0].NetworkOn {
		t.Errorf("wrapped = %+v", sb.wrapped)
	}
}

func TestRun_CapsOutputWithoutBlockingTheProgram(t *testing.T) {
	prog := script(t, `i=0; while [ $i -lt 2000 ]; do echo "0123456789"; i=$((i+1)); done`)
	inv := invocation(t)
	inv.MaxOutput = 100

	out, err := confined().Run(context.Background(), prog, inv)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out.Stdout) != 100 || !out.Truncated {
		t.Errorf("got %d bytes, truncated=%v", len(out.Stdout), out.Truncated)
	}
}

func TestRun_KillsTheWholeProcessGroupOnTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups")
	}
	marker := filepath.Join(t.TempDir(), "child-survived")
	// The shell backgrounds a child that would write the marker after the timeout; killing only the shell would let it.
	prog := script(t, `(sleep 1; touch `+marker+`) & sleep 30`)
	inv := invocation(t)
	inv.Timeout = 200 * time.Millisecond

	start := time.Now()
	_, err := confined().Run(context.Background(), prog, inv)
	if !errors.Is(err, ErrTimedOut) {
		t.Fatalf("err = %v, want ErrTimedOut", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("the run must end at the timeout, took %s", time.Since(start))
	}
	time.Sleep(1500 * time.Millisecond)
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("a descendant outlived the timeout: the process group was not killed")
	}
}

func TestRun_ReportsAFailingProgramWithItsStderrLine(t *testing.T) {
	prog := script(t, `echo "boom: bad flag" >&2; exit 3`)
	_, err := confined().Run(context.Background(), prog, invocation(t))
	if err == nil || !strings.Contains(err.Error(), "boom: bad flag") {
		t.Errorf("err = %v", err)
	}
}

func TestDetect(t *testing.T) {
	prog := script(t, `echo "fakeprog 1.2.3 ( https://example.test )"; echo second`)
	got, err := Detect(context.Background(), prog)
	if err != nil || got.Version != "fakeprog 1.2.3 ( https://example.test )" || got.Path != prog.Binary {
		t.Errorf("Detect = %+v, %v", got, err)
	}

	missing := Program{Name: "nope", Binary: "definitely-not-installed-xyz", InstallHint: map[string]string{runtime.GOOS: "do the install"}}
	_, err = Detect(context.Background(), missing)
	var notInstalled *NotInstalledError
	if !errors.As(err, &notInstalled) || !strings.Contains(err.Error(), "do the install") {
		t.Errorf("err = %v, want a NotInstalledError with the hint", err)
	}
}
