// Package sandbox defines the host-level OS-confinement seam for agent-spawned
// child processes (agent-os-sandboxing plan Phase 2). It is the S4 spawn
// backend: one Provider, selected at bootstrap from config + capability
// detection, applied at the single child-spawn choke point (pooled shell and
// executeLocal via the R1 AgentCommand helper).
//
// Mechanisms: Linux uses Landlock (filesystem jail on every supported kernel;
// TCP bind/connect denial for network-off children on ABI v4+/kernel >= 6.7).
// macOS has no shipped mechanism yet — containment there is uid-first (the
// launchd dedicated-user deployment, plan D4); Seatbelt is not built.
//
// Downgrade contract (SPEC-006 §II.7): a Provider must never claim enforcement
// it does not apply. When the requested surface has no mechanism on this host,
// New returns a provider whose EffectiveState reports request-vs-actual with a
// reason — enforcement is never silent and never assumed.
package sandbox

import (
	"fmt"
	"os/exec"
)

// Enforcement is the OS mechanism confining a surface.
type Enforcement string

const (
	EnforcementNone     Enforcement = "none"
	EnforcementLandlock Enforcement = "landlock"
)

// networkGapReason is the SINGLE reason string for a provider that applies no
// OS network deny (no-op provider, macOS, or Landlock kernels below ABI v4):
// network is then enforced at the Go layer (schema/grants) plus the egress
// proxy. Used by the no-op provider and any OS provider's Effective() so
// reporting never drifts.
const networkGapReason = "OS network deny not applied by this provider (host/ABI lacks it or filesystem jail inactive); Go layer + egress proxy enforce"

// SurfaceState reports what a surface enforces and why.

type SurfaceState struct {
	Mechanism Enforcement `json:"mechanism"`
	Reason    string      `json:"reason,omitempty"`
}

// EffectiveState is the read-only request-vs-actual projection surfaced in the
// UI and logs. It is a runtime projection, never persisted.
type EffectiveState struct {
	Filesystem SurfaceState `json:"filesystem"`
	Network    SurfaceState `json:"network"`
	Provider   string       `json:"provider"`
}

// String renders Effective for logs (downgrades must be visible, not silent).
func (e EffectiveState) String() string {
	return fmt.Sprintf("provider=%s filesystem=%s network=%s", e.Provider, surface(e.Filesystem), surface(e.Network))
}

func surface(s SurfaceState) string {
	if s.Mechanism == EnforcementNone && s.Reason != "" {
		return "none (" + s.Reason + ")"
	}
	return string(s.Mechanism)
}

// WorkspaceView carries everything a spawn needs to derive its jail grants.
type WorkspaceView struct {
	WorkspaceID string // "" for non-workspace one-shots (executeLocal)
	RootPath    string // workspace absolute path (grant root)
	NetworkOn   bool   // OS network state for this process (D8)
	Epoch       int64  // policy epoch the spawn was resolved under
}

// Provider wraps a prepared exec.Cmd so the child runs under the OS sandbox.
type Provider interface {
	// Wrap mutates cmd (argv, env, SysProcAttr) so the child is confined to ws.
	// Safe to call more than once on equivalent (cmd, ws) pairs.
	Wrap(cmd *exec.Cmd, ws WorkspaceView) error
	// Effective reports what this host actually enforces.
	Effective() EffectiveState
}

// Config is the host request: which surfaces the operator asked to confine.
type Config struct {
	Filesystem bool
	// MaxStorageGB > 0 arms the per-file kernel backstop (RLIMIT_FSIZE) for
	// jailed children — the plan's storage accounting boundary, applied in the
	// runner before the real argv execs. 0 disables it.
	MaxStorageGB int
}

// New selects the Provider for the host from config + capability detection.
// When the requested surface has a real mechanism it is returned (Linux:
// Landlock); otherwise the no-op provider reports the downgrade honestly
// (SPEC-006 §II.7) — requested-but-unavailable surfaces say "none (reason)",
// disabled surfaces say "disabled by config". Enforcement is never silent and
// never assumed.
func New(cfg Config) Provider {
	fsReason := "disabled by config"
	netReason := "disabled by config"
	if cfg.Filesystem {
		fp, reason := newFilesystemProvider(cfg)
		if fp != nil {
			return fp
		}
		fsReason = reason
		// OS TCP deny is expressed only by the filesystem jail provider (for
		// jailed network-off children), so an unavailable jail means network is
		// not OS-enforced either; the Go layer + egress proxy still apply.
		netReason = networkGapReason
	}
	return &noop{
		effective: EffectiveState{
			Filesystem: SurfaceState{Mechanism: EnforcementNone, Reason: fsReason},
			Network:    SurfaceState{Mechanism: EnforcementNone, Reason: netReason},
			Provider:   "noop",
		},
	}
}

// noop is the identity Provider: it applies no confinement and reports exactly
// that. Wrapping with it must be a no-op so the spawn path is identical
// whether or not an OS mechanism exists (filesystem:false keeps one code path).
type noop struct {
	effective EffectiveState
}

func (p *noop) Wrap(_ *exec.Cmd, _ WorkspaceView) error { return nil }

func (p *noop) Effective() EffectiveState { return p.effective }
