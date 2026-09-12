package models

import (
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Default posture for the agent-os-sandboxing plan (D1/D5/D6): fresh installs
// ship with the filesystem jail ON and agent network OFF, both written as
// explicit values; MaxMemoryMB default is recalibrated from the known-broken
// 256 (node/go reserve multi-GB of virtual address space — see plan §3).
func TestDefaultHostSettingsSandboxingPosture(t *testing.T) {
	s := DefaultHostSettings().Sandboxing
	if !s.Enabled {
		t.Error("default sandboxing.enabled must be true")
	}
	if s.Filesystem != nil {
		t.Error("default filesystem must be nil (nil resolves to ON — no explicit key needed)")
	}
	if !s.FilesystemEnabled() {
		t.Error("default filesystem jail must be ON")
	}
	if s.NetworkAllowed() {
		t.Error("fresh-install default must have agent network OFF (D1)")
	}
	if !s.NetworkDecided() {
		t.Error("fresh-install default must carry an explicit network value, never undecided")
	}
	if s.MaxMemoryMB != 2048 {
		t.Errorf("default MaxMemoryMB = %d, want 2048 (256 breaks node/go)", s.MaxMemoryMB)
	}
	if s.EgressProxy != 0 {
		t.Errorf("default egress_proxy = %d, want 0 (off)", s.EgressProxy)
	}
}

// The additive-config hazard (D6): a settings.yml that predates the new keys
// (Filesystem/Network absent → nil) must boot with the jail ON and network in
// its legacy ALLOWED state — never silently off — while remaining decidable by
// the operator (NetworkDecided=false drives the UI migration banner).
func TestHostSandboxingUndecidedSemantics(t *testing.T) {
	c := HostSandboxingConfig{Enabled: true, MaxMemoryMB: 256} // legacy shape
	if !c.FilesystemEnabled() {
		t.Error("nil Filesystem must resolve to ON (today's effective behavior preserved)")
	}
	if !c.NetworkAllowed() {
		t.Error("nil Network must resolve to ALLOWED on upgrade (legacy behavior preserved until operator decides)")
	}
	if c.NetworkDecided() {
		t.Error("nil Network must report undecided so the migration banner can show")
	}
}

func TestHostSandboxingExplicitOff(t *testing.T) {
	fsOn, netOff := true, false
	c := HostSandboxingConfig{Enabled: true, Filesystem: &fsOn, Network: &netOff}
	if !c.FilesystemEnabled() {
		t.Error("explicit filesystem:true must resolve ON")
	}
	if c.NetworkAllowed() {
		t.Error("explicit network:false must resolve OFF (hard ceiling)")
	}
	if !c.NetworkDecided() {
		t.Error("explicit network:false must be decided")
	}
}

// yaml.v3 lowercases field names and ignores json tags (verified 2026-09-06),
// so legacy on-disk keys are enabled/maxstoragegb/maxmemorymb/functional. The
// yaml tags must keep those legacy keys byte-identical and add explicit
// filesystem/network/egress_proxy keys for the new fields.
func TestHostSandboxingYAMLKeys(t *testing.T) {
	fsOn, netOff := true, false
	c := HostSandboxingConfig{
		Enabled: true, MaxStorageGB: 2, MaxMemoryMB: 256,
		Filesystem: &fsOn, Network: &netOff, EgressProxy: 4002,
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{
		"enabled: true", "maxstoragegb: 2", "maxmemorymb: 256",
		"filesystem: true", "network: false", "egress_proxy: 4002",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("marshaled yaml missing %q:\n%s", want, data)
		}
	}

	// A legacy document (lowercased field-name keys, no new keys) must unmarshal
	// with Filesystem/Network left nil (undecided), not false.
	var got HostSandboxingConfig
	legacy := "enabled: true\nmaxstoragegb: 2\nmaxmemorymb: 256\nfunctional: false\n"
	if err := yaml.Unmarshal([]byte(legacy), &got); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if !got.Enabled {
		t.Error("legacy enabled:true lost")
	}
	if got.MaxStorageGB != 2 || got.MaxMemoryMB != 256 {
		t.Errorf("legacy numeric keys lost: %+v", got)
	}
	if got.Filesystem != nil || got.Network != nil {
		t.Errorf("absent new keys must unmarshal to nil (undecided), got %+v", got)
	}
}

// NetworkScope (plan §4.4 grant model, L2 automation grants).
func TestNetworkScopeEnum(t *testing.T) {
	for _, s := range []NetworkScope{NetworkScopeInherit, NetworkScopeNone, NetworkScopeLan, NetworkScopeInternet} {
		if !s.Valid() {
			t.Errorf("%q must be a valid scope", s)
		}
	}
	if NetworkScope("bogus").Valid() {
		t.Error("unknown scope must be invalid")
	}
	if NetworkScopeNone.NetworkOn() || NetworkScopeInherit.NetworkOn() {
		t.Error("none/inherit must not imply network on")
	}
	if !NetworkScopeLan.NetworkOn() || !NetworkScopeInternet.NetworkOn() {
		t.Error("lan/internet must imply network on")
	}
}

func TestNetworkGuardrailsEffectiveScope(t *testing.T) {
	wsInternet := NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, AllowInternetAccess: true}
	wsInternetOnly := NetworkGuardrailsConfig{Enabled: true, AllowInternetAccess: true}
	wsLan := NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true}
	wsOff := NetworkGuardrailsConfig{Enabled: false}

	tests := []struct {
		name        string
		hostAllowed bool
		ws          NetworkGuardrailsConfig
		grant       NetworkScope
		want        NetworkScope
	}{
		{"host off beats explicit internet grant", false, wsInternet, NetworkScopeInternet, NetworkScopeNone},
		{"explicit none wins over ws internet", true, wsInternet, NetworkScopeNone, NetworkScopeNone},
		{"explicit lan grants in offline ws (L2 loosens, host is ceiling)", true, wsOff, NetworkScopeLan, NetworkScopeLan},
		{"explicit internet wins over ws lan", true, wsLan, NetworkScopeInternet, NetworkScopeInternet},
		{"inherit follows ws internet", true, wsInternet, NetworkScopeInherit, NetworkScopeInternet},
		{"inherit follows ws internet-only (lan off)", true, wsInternetOnly, NetworkScopeInherit, NetworkScopeInternetOnly},
		{"explicit internet_only grant wins over ws internet", true, wsInternet, NetworkScopeInternetOnly, NetworkScopeInternetOnly},
		{"inherit follows ws lan", true, wsLan, NetworkScopeInherit, NetworkScopeLan},
		{"inherit with ws disabled is none", true, wsOff, NetworkScopeInherit, NetworkScopeNone},
		{"unknown grant treated as inherit", true, wsLan, NetworkScope("bogus"), NetworkScopeLan},
		{"host on empty ws and no grant is none", true, NetworkGuardrailsConfig{}, NetworkScopeInherit, NetworkScopeNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ws.EffectiveScope(tt.hostAllowed, tt.grant); got != tt.want {
				t.Errorf("EffectiveScope(%v, %q) = %q, want %q", tt.hostAllowed, tt.grant, got, tt.want)
			}
		})
	}
}

func TestAutomationNetworkGrantYAML(t *testing.T) {
	// Empty grant (inherit) is omitted; explicit values round-trip.
	a := Automation{Name: "scan"}
	data, err := yaml.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "network_grant") {
		t.Errorf("inherit grant must be omitted from yaml:\n%s", data)
	}
	a2 := Automation{Name: "scan", NetworkGrant: NetworkScopeInternet}
	data2, err := yaml.Marshal(a2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data2), "network_grant: internet") {
		t.Errorf("explicit grant must serialize:\n%s", data2)
	}
	var got Automation
	if err := yaml.Unmarshal(data2, &got); err != nil {
		t.Fatal(err)
	}
	if got.NetworkGrant != NetworkScopeInternet {
		t.Errorf("round-trip = %q, want internet", got.NetworkGrant)
	}
}

// Run-scope ctx plumbing (plan §4.4/D8): consumers read the stamped scope via
// RunNetworkScopeFrom; absent or inherit values report ok=false so callers fall
// back to the workspace guardrail tier (pre-grant behavior).
func TestRunNetworkScopeContext(t *testing.T) {
	ctx := context.Background()
	if s, ok := RunNetworkScopeFrom(ctx); ok || s != NetworkScopeInherit {
		t.Errorf("bare ctx: got (%q, %v), want (\"\", false)", s, ok)
	}
	ctx = WithRunNetworkScope(ctx, NetworkScopeInternet)
	if s, ok := RunNetworkScopeFrom(ctx); !ok || s != NetworkScopeInternet {
		t.Errorf("stamped ctx: got (%q, %v)", s, ok)
	}
	ctx = WithRunNetworkScope(ctx, NetworkScopeInherit)
	if s, ok := RunNetworkScopeFrom(ctx); ok || s != NetworkScopeInherit {
		t.Errorf("inherit stamp must read as absent: got (%q, %v)", s, ok)
	}
	ctx = WithRunNetworkScope(ctx, NetworkScope("bogus"))
	if s, ok := RunNetworkScopeFrom(ctx); ok || s != NetworkScopeInherit {
		t.Errorf("invalid stamp must read as absent: got (%q, %v)", s, ok)
	}
}

// Operator egress domain policy (plan §4.5/1d): allow list non-empty ⇒ default
// deny; deny applies always; lists round-trip through yaml.
func TestEgressPolicyAndYAML(t *testing.T) {
	allow, deny, dd := (HostSandboxingConfig{}).EgressPolicy()
	if dd || len(allow) != 0 || len(deny) != 0 {
		t.Errorf("empty config must be allow-all: allow=%v deny=%v defaultDeny=%v", allow, deny, dd)
	}

	c := HostSandboxingConfig{
		EgressAllowDomains: []string{".telegram.org", "api.openai.com"},
		EgressDenyDomains:  []string{"evil.example.com"},
	}
	allow, deny, dd = c.EgressPolicy()
	if !dd {
		t.Error("non-empty allow list must default-deny")
	}
	if len(allow) != 2 || len(deny) != 1 {
		t.Errorf("policy lists wrong: allow=%v deny=%v", allow, deny)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "egress_allow_domains:") || !strings.Contains(out, "api.openai.com") {
		t.Errorf("allow domains not serialized:\n%s", out)
	}
	if !strings.Contains(out, "egress_deny_domains:") || !strings.Contains(out, "evil.example.com") {
		t.Errorf("deny domains not serialized:\n%s", out)
	}

	var got HostSandboxingConfig
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.EgressAllowDomains) != 2 || len(got.EgressDenyDomains) != 1 {
		t.Errorf("round-trip failed: %+v", got)
	}
}

// WithRunScope centralizes the scope→virtual-config mapping used by both the
// guardrail validator and the runtime network-tool config (single source —
// plan §4.4/D7, no layer drift).
func TestNetworkGuardrailsWithRunScope(t *testing.T) {
	base := NetworkGuardrailsConfig{Enabled: false}

	vcfg, ok := base.WithRunScope(NetworkScopeNone)
	if !ok || vcfg.Enabled {
		t.Errorf("scope none must disable network tools, got %+v ok=%v", vcfg, ok)
	}
	vcfg, ok = base.WithRunScope(NetworkScopeLan)
	if !ok || !vcfg.Enabled || !vcfg.AllowLanAccess || vcfg.AllowInternetAccess {
		t.Errorf("scope lan must enable LAN only, got %+v", vcfg)
	}
	vcfg, ok = base.WithRunScope(NetworkScopeInternet)
	if !ok || !vcfg.Enabled || !vcfg.AllowLanAccess || !vcfg.AllowInternetAccess {
		t.Errorf("scope internet must enable LAN+internet, got %+v", vcfg)
	}
	// Inherit/unknown scopes leave the config untouched (ok=false) so callers
	// fall back to the merged workspace tier.
	if vcfg, ok := base.WithRunScope(NetworkScopeInherit); ok || vcfg.Enabled {
		t.Errorf("inherit must not scope the config: ok=%v cfg=%+v", ok, vcfg)
	}
	if vcfg, ok := base.WithRunScope(NetworkScope("bogus")); ok || vcfg.Enabled {
		t.Errorf("unknown scope must not scope the config: ok=%v cfg=%+v", ok, vcfg)
	}
	// Blocked-domain/blocked-IP lists survive a scope override (restrictions
	// compose with the allow flags).
	withList := base
	withList.BlockedDomains = []string{"evil.example"}
	vcfg, _ = withList.WithRunScope(NetworkScopeInternet)
	if len(vcfg.BlockedDomains) != 1 || vcfg.BlockedDomains[0] != "evil.example" {
		t.Errorf("scope override must preserve blocked-domain lists, got %+v", vcfg)
	}
}
