package models

import "gopkg.in/yaml.v3"

// HostSettings is the host-level settings document (settings.yml → host view).
// Effective is a READ-TIME RUNTIME PROJECTION of OS enforcement (SPEC-006
// §II.7.4 downgrade-never-bypass), stamped onto the copy returned by the host
// settings facade — it is never persisted into settings.yml, which stores only
// the requested policy.
type HostSettings struct {
	Sandboxing HostSandboxingConfig `json:"sandboxing"`
	// Effective reports what the running host actually enforces vs what
	// sandboxing requests (mechanism per surface, with a reason when the
	// request cannot be met). nil until the sandbox provider is wired at
	// startup. Mirrors platform/sandbox.EffectiveState in a leaf-safe shape.
	Effective *SandboxEffective `json:"effective,omitempty"`
}

// SandboxEffective is the JSON-safe runtime projection of OS enforcement. It
// deliberately mirrors platform/sandbox.EffectiveState instead of importing it,
// keeping the models leaf free of platform dependencies.
type SandboxEffective struct {
	Filesystem SandboxSurface `json:"filesystem"`
	Network    SandboxSurface `json:"network"`
	Provider   string         `json:"provider"`
}

// SandboxSurface reports one enforced surface: the mechanism (""/none or the
// mechanism name) and, when the requested enforcement is not available, why.
type SandboxSurface struct {
	Mechanism string `json:"mechanism"`
	Reason    string `json:"reason,omitempty"`
}

// HostSandboxingConfig is the host-level sandboxing policy (settings.yml →
// sandboxing:). Enabled gates terminal availability today and becomes the OS-jail
// master (agent-os-sandboxing plan rev 2); Filesystem/Network are host-level hard
// gates added as *bool so a settings.yml that predates them keeps today's
// behavior instead of booting with the jail off and network off (plan D6 — the
// additive-config hazard: yaml.v3 lowercases field names, so a plain bool would
// unmarshal absent keys to false).
//
// On-disk key note: this section predates yaml tags; yaml.v3 lowercases field
// names, so legacy files use enabled/maxstoragegb/maxmemorymb/functional. The
// yaml tags below keep those keys byte-identical and add explicit
// filesystem/network/egress_proxy keys for the new fields.
type HostSandboxingConfig struct {
	Enabled      bool `yaml:"enabled" json:"enabled"`
	MaxStorageGB int  `yaml:"maxstoragegb" json:"max_storage_gb"`
	MaxMemoryMB  int  `yaml:"maxmemorymb" json:"max_memory_mb"`
	Functional   bool `yaml:"functional" json:"functional"`
	// Filesystem: OS-level FS jail (Landlock/seatbelt) for agent children. nil =
	// undecided → ON (today's effective behavior; the jail adds workspace↔workspace
	// isolation on Linux-prod and host-secret containment on dev/desktop runs).
	Filesystem *bool `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
	// Network: HOST MASTER / hard ceiling for agent network (plan D1, R4). nil =
	// undecided → ALLOWED (legacy upgrade behavior preserved until the operator
	// writes an explicit value; UI migration banner keys off NetworkDecided).
	// Explicit false = no workspace grant or persisted approval can open network
	// for agent activity — enforced OUTSIDE the guardrail MergeWith chain.
	Network *bool `yaml:"network,omitempty" json:"network,omitempty"`
	// EgressProxy: 0 = off; a non-zero port enables the local egress proxy that
	// applies domain/CIDR policy to all agent egress (plan D2, Phase 1).
	EgressProxy int `yaml:"egress_proxy" json:"egress_proxy"`
	// Operator domain policy for the egress proxy (plan §4.5/1d). When
	// EgressAllowDomains is non-empty the proxy becomes default-deny (only
	// listed hosts, exact or ".suffix"); EgressDenyDomains wins over allow and
	// applies regardless. Absent (nil) = allow-all, preserving pre-policy
	// behavior.
	EgressAllowDomains []string `yaml:"egress_allow_domains,omitempty" json:"egress_allow_domains,omitempty"`
	EgressDenyDomains  []string `yaml:"egress_deny_domains,omitempty" json:"egress_deny_domains,omitempty"`

	// SectionPresent records that the on-disk document actually contained a
	// `sandboxing:` block. It is decode-only state: never written to YAML
	// (yaml:"-"), and JSON-visible solely because the config store deep-copies
	// payloads through a JSON round-trip. It lets the loader distinguish
	// "section absent → take defaults" from "explicitly enabled:false" — the
	// latter must survive a reload instead of being merged back to defaults.
	// The host-settings API projection clears it.
	SectionPresent bool `yaml:"-" json:"section_present,omitempty"`
}

// UnmarshalYAML marks the section present before decoding into the plain
// struct shape (avoids recursion via the raw alias type).
func (c *HostSandboxingConfig) UnmarshalYAML(value *yaml.Node) error {
	type raw HostSandboxingConfig
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	*c = HostSandboxingConfig(r)
	c.SectionPresent = true
	return nil
}

// FilesystemEnabled resolves the effective filesystem-jail state: undecided
// (nil) counts as ON so pre-change installs keep today's confinement semantics.
func (c HostSandboxingConfig) FilesystemEnabled() bool {
	return c.Filesystem == nil || *c.Filesystem
}

// EgressPolicy builds the egress proxy's host policy from operator config:
// non-empty allow list ⇒ default-deny (only listed hosts reachable); deny wins.
// Allow-all when no allow list is configured. The lists are copied so a caller
// cannot mutate persisted config through the returned slices.
func (c HostSandboxingConfig) EgressPolicy() (allow, deny []string, defaultDeny bool) {
	return append([]string(nil), c.EgressAllowDomains...),
		append([]string(nil), c.EgressDenyDomains...),
		len(c.EgressAllowDomains) > 0
}

// NetworkAllowed resolves the effective network state for runtime enforcement:
// undecided (nil) counts as ALLOWED so legacy installs keep today's behavior
// until the operator writes an explicit value (see NetworkDecided).
func (c HostSandboxingConfig) NetworkAllowed() bool {
	return c.Network == nil || *c.Network
}

// NetworkDecided reports whether the operator has written an explicit network
// value. False drives the UI migration banner ("agent network is currently
// unrestricted — restrict?").
func (c HostSandboxingConfig) NetworkDecided() bool {
	return c.Network != nil
}

func DefaultHostSettings() HostSettings {
	return HostSettings{
		Sandboxing: HostSandboxingConfig{
			Enabled:      true,
			MaxStorageGB: 2,
			// Recalibrated from the historically decorative 256: node/go toolchains
			// reserve multi-GB of virtual address space and die under a tight
			// RLIMIT_AS/RLIMIT_DATA cap (measured — plan §3). Phase 3 enforces this.
			MaxMemoryMB: 2048,
			// Filesystem is intentionally nil: nil resolves to ON (FilesystemEnabled),
			// and absent→ON means the same for fresh AND legacy installs — no explicit
			// key needed (unlike Network, where absent must mean legacy-allowed but
			// fresh installs must be off, so the OFF default is written explicitly).
			Network: new(bool), // explicit OFF (zero value) — D1 default-off
		},
	}
}
