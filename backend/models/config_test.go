package models

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestMergeWithNetworkOverridePresence verifies the override-stack contract for
// the network block (SPEC-006 §II.2): a layer that explicitly configured the
// block replaces the inherited allow flags — including an explicit `false` that
// restricts — while a layer that omitted the block inherits the baseline.
func TestMergeWithNetworkOverridePresence(t *testing.T) {
	tests := []struct {
		name          string
		global        NetworkGuardrailsConfig
		workspaceYAML string
		wantScope     NetworkScope
	}{
		{
			name:          "workspace restricts internet to lan",
			global:        NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, AllowInternetAccess: true},
			workspaceYAML: "network:\n  enabled: true\n  allowlanaccess: true\n  allowinternetaccess: false\n",
			wantScope:     NetworkScopeLan,
		},
		{
			name:          "workspace blocks lan keeps internet",
			global:        NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, AllowInternetAccess: true},
			workspaceYAML: "network:\n  enabled: true\n  allowlanaccess: false\n  allowinternetaccess: true\n",
			wantScope:     NetworkScopeInternetOnly,
		},
		{
			name:          "workspace disables network entirely",
			global:        NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, AllowInternetAccess: true},
			workspaceYAML: "network:\n  enabled: false\n  allowlanaccess: false\n  allowinternetaccess: false\n",
			wantScope:     NetworkScopeNone,
		},
		{
			name:          "workspace loosens within baseline",
			global:        NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, AllowInternetAccess: false},
			workspaceYAML: "network:\n  enabled: true\n  allowlanaccess: true\n  allowinternetaccess: true\n",
			wantScope:     NetworkScopeInternet,
		},
		{
			name:          "absent workspace network inherits global",
			global:        NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, AllowInternetAccess: true},
			workspaceYAML: "cron_schedule: \"\"\n",
			wantScope:     NetworkScopeInternet,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := AgentGuardrailsConfig{Network: tt.global}
			var ws AgentGuardrailsConfig
			if err := yaml.Unmarshal([]byte(tt.workspaceYAML), &ws); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			base.MergeWith(&ws)
			if got := base.Network.EffectiveScope(true, NetworkScopeInherit); got != tt.wantScope {
				t.Fatalf("effective scope = %q, want %q (merged network = %+v)", got, tt.wantScope, base.Network)
			}
		})
	}
}

// TestWithRunScopeMapping pins the scope→config mapping (single source shared
// by the validator and the runtime tools), including the independent
// internet-only scope (internet on, LAN off).
func TestWithRunScopeMapping(t *testing.T) {
	tests := []struct {
		scope        NetworkScope
		wantChanged  bool
		wantEnabled  bool
		wantLan      bool
		wantInternet bool
	}{
		{scope: NetworkScopeInherit, wantChanged: false},
		{scope: NetworkScopeNone, wantChanged: true, wantEnabled: false, wantLan: true, wantInternet: true},
		{scope: NetworkScopeLan, wantChanged: true, wantEnabled: true, wantLan: true, wantInternet: false},
		{scope: NetworkScopeInternetOnly, wantChanged: true, wantEnabled: true, wantLan: false, wantInternet: true},
		{scope: NetworkScopeInternet, wantChanged: true, wantEnabled: true, wantLan: true, wantInternet: true},
	}
	base := NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, AllowInternetAccess: true}
	for _, tt := range tests {
		name := string(tt.scope)
		if name == "" {
			name = "inherit"
		}
		t.Run(name, func(t *testing.T) {
			got, changed := base.WithRunScope(tt.scope)
			if changed != tt.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if !tt.wantChanged {
				return
			}
			if got.Enabled != tt.wantEnabled || got.AllowLanAccess != tt.wantLan || got.AllowInternetAccess != tt.wantInternet {
				t.Fatalf("got enabled=%v lan=%v internet=%v, want enabled=%v lan=%v internet=%v",
					got.Enabled, got.AllowLanAccess, got.AllowInternetAccess, tt.wantEnabled, tt.wantLan, tt.wantInternet)
			}
		})
	}
}

// TestNetworkScopeValidityAndNetworkOn guards the enum: internet_only is valid
// and counts as network-on for the shell pool key (D8).
func TestNetworkScopeValidityAndNetworkOn(t *testing.T) {
	tests := []struct {
		scope NetworkScope
		valid bool
		on    bool
	}{
		{NetworkScopeInherit, true, false},
		{NetworkScopeNone, true, false},
		{NetworkScopeLan, true, true},
		{NetworkScopeInternetOnly, true, true},
		{NetworkScopeInternet, true, true},
		{NetworkScope("bogus"), false, false},
	}
	for _, tt := range tests {
		if got := tt.scope.Valid(); got != tt.valid {
			t.Errorf("%q.Valid() = %v, want %v", tt.scope, got, tt.valid)
		}
		if got := tt.scope.NetworkOn(); got != tt.on {
			t.Errorf("%q.NetworkOn() = %v, want %v", tt.scope, got, tt.on)
		}
	}
}

// TestWorkspaceConfigNetworkPresenceDecoded guards the real read path
// ({workspace}/config.yaml -> WorkspaceConfig): the network block must be
// observed as present so MergeWith can honor an explicit `false`.
func TestWorkspaceConfigNetworkPresenceDecoded(t *testing.T) {
	var cfg WorkspaceConfig
	const doc = "guardrails:\n  network:\n    enabled: true\n    allowlanaccess: true\n    allowinternetaccess: false\n"
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if cfg.Guardrails == nil {
		t.Fatal("guardrails not decoded")
	}
	if !cfg.Guardrails.Network.present {
		t.Fatal("network block presence not recorded on decode")
	}
}
