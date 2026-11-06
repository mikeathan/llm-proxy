//go:build linux

package sandbox

import (
	"os/exec"
	"strings"
	"testing"
)

func wrapped(t *testing.T, p Provider, ws WorkspaceView) []string {
	t.Helper()
	ws.RootPath = t.TempDir()
	cmd := exec.Command("true")
	cmd.Env = []string{"PATH=/usr/bin"}
	if err := p.Wrap(cmd, ws); err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	return cmd.Env
}

func hasMarker(env []string) bool {
	for _, kv := range env {
		if kv == unixDenyEnvVar+"=1" {
			return true
		}
	}
	return false
}

// A child gets no Unix sockets unless the spawn site or the operator says it may: the zero value of every switch is "denied".
func TestWrap_DeniesUnixSocketsUnlessSomeoneAllowsThem(t *testing.T) {
	if _, err := landlockABI(); err != nil {
		t.Skipf("Landlock unavailable: %v", err)
	}
	if !unixDenySupported() {
		t.Skip("this architecture cannot express the Unix-socket denial")
	}
	if env := wrapped(t, landlockProvider{}, WorkspaceView{}); !hasMarker(env) {
		t.Error("a default spawn must carry the denial")
	}
	if env := wrapped(t, landlockProvider{}, WorkspaceView{AllowUnixSockets: true}); hasMarker(env) {
		t.Error("a spawn site that needs Unix sockets (a program that talks to D-Bus) must be able to allow them for itself")
	}
	if env := wrapped(t, landlockProvider{allowUnixSockets: true}, WorkspaceView{}); hasMarker(env) {
		t.Error("the operator's setting must lift the denial for every child")
	}
}

func TestEffective_SaysWhetherUnixSocketsAreDenied(t *testing.T) {
	if _, err := landlockABI(); err != nil {
		t.Skipf("Landlock unavailable: %v", err)
	}
	deny := landlockProvider{}.Effective().UnixSockets
	allow := landlockProvider{allowUnixSockets: true}.Effective().UnixSockets
	if unixDenySupported() {
		if deny.Mechanism != EnforcementSeccomp {
			t.Errorf("denied surface = %+v, want seccomp", deny)
		}
	} else if deny.Mechanism != EnforcementNone || deny.Reason == "" {
		t.Errorf("an architecture that cannot deny must say so, got %+v", deny)
	}
	if allow.Mechanism != EnforcementNone || !strings.Contains(allow.Reason, "allowed") {
		t.Errorf("allowed surface = %+v, want none with the reason that the operator allows them", allow)
	}
}
