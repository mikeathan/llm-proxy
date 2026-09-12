package sandbox

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The downgrade contract (SPEC-006 §II.7 / plan §4.2): EffectiveState must
// report request-vs-actual truthfully — requested-but-unavailable surfaces
// report "none (reason)", disabled surfaces say so, and the no-op provider
// applies no confinement.
func TestNewDisabledByConfig(t *testing.T) {
	p := New(Config{})
	eff := p.Effective()
	if eff.Filesystem.Mechanism != EnforcementNone || eff.Network.Mechanism != EnforcementNone {
		t.Fatalf("disabled surfaces must report none: %+v", eff)
	}
	if eff.Filesystem.Reason != "disabled by config" || eff.Network.Reason != "disabled by config" {
		t.Errorf("disabled surfaces must say 'disabled by config': %+v", eff)
	}
	if eff.Provider != "noop" {
		t.Errorf("provider = %q, want noop", eff.Provider)
	}
	// A requested-but-unavailable surface must not be silent.
	if err := p.Wrap(&exec.Cmd{}, WorkspaceView{}); err != nil {
		t.Errorf("noop Wrap must be a no-op, got %v", err)
	}
}

func TestNewRequestedButUnavailable(t *testing.T) {
	if filesystemMechanismAvailable() {
		// A real mechanism is selected on this host (Linux Landlock) — the
		// no-op downgrade path is exercised by the !linux builds and by the
		// mechanism-specific Linux tests. Skipping keeps the promised Linux CI
		// (Landlock runtime probes) green.
		t.Skip("filesystem mechanism available on this host — no-op downgrade not exercised here")
	}
	p := New(Config{Filesystem: true})
	eff := p.Effective()
	if eff.Filesystem.Mechanism != EnforcementNone {
		t.Errorf("no mechanism on this build: filesystem must report none, got %s", eff.Filesystem.Mechanism)
	}
	if eff.Filesystem.Reason == "" || strings.Contains(eff.Filesystem.Reason, "disabled") {
		t.Errorf("requested-but-unavailable must give the real reason, got %q", eff.Filesystem.Reason)
	}
	if !strings.Contains(eff.String(), "none (") {
		t.Errorf("String must surface the downgrade visibly, got %q", eff.String())
	}
}

func TestEffectiveString(t *testing.T) {
	if filesystemMechanismAvailable() {
		t.Skip("filesystem mechanism available on this host — no-op provider not selected")
	}
	p := New(Config{Filesystem: true})
	s := p.Effective().String()
	for _, want := range []string{"provider=noop", "filesystem=", "network="} {
		if !strings.Contains(s, want) {
			t.Errorf("Effective.String() missing %q: %s", want, s)
		}
	}
}

// The runner serialization must be stable and lossless (decoded on Linux by
// DecodeProfile). EncodeProfile runs on all OSes.
func TestEncodeProfile(t *testing.T) {
	p := Profile{Rules: []Rule{
		{Path: "/w", Perm: PermRead | PermWrite},
		{Path: "/usr/bin", Perm: PermRead},
		{Path: "/", Perm: PermTraverse},
	}}
	enc := EncodeProfile(p)
	want := "3 /w\n1 /usr/bin\n4 /\n"
	if enc != want {
		t.Errorf("EncodeProfile = %q, want %q", enc, want)
	}
}

// TestEffectiveMatchesExpectedHost is the CI conformance gate for capability
// detection (plan §9 T2): with LLMPROXY_EXPECT_FS set, the host MUST actually
// select the expected mechanism. A silent downgrade (e.g. the Landlock probe
// failing on a kernel that supports it) fails the job instead of skipping —
// otherwise the whole OS-enforcement matrix could skip while CI stays green.
func TestEffectiveMatchesExpectedHost(t *testing.T) {
	expectFS := os.Getenv("LLMPROXY_EXPECT_FS")
	if expectFS == "" {
		t.Skip("LLMPROXY_EXPECT_FS not set (CI sandbox-conformance job only)")
	}
	eff := New(Config{Filesystem: true}).Effective()

	switch expectFS {
	case "none":
		if eff.Filesystem.Mechanism != EnforcementNone || eff.Provider != "noop" {
			t.Fatalf("expected no-op provider on this host, got %+v", eff)
		}
	case "landlock":
		if eff.Filesystem.Mechanism != EnforcementLandlock || eff.Provider != "landlock" {
			t.Fatalf("expected the Landlock provider on this host, got %+v", eff)
		}
	default:
		t.Fatalf("unknown LLMPROXY_EXPECT_FS %q (want none|landlock)", expectFS)
	}

	wantNetDeny := os.Getenv("LLMPROXY_EXPECT_NET_DENY") == "1"
	gotNetDeny := eff.Network.Mechanism == EnforcementLandlock
	if gotNetDeny != wantNetDeny {
		t.Fatalf("OS network deny = %v (%+v), want %v", gotNetDeny, eff.Network, wantNetDeny)
	}
}
