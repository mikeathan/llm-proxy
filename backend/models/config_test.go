package models

import (
	"errors"
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

// TestSearchProviderValid pins the enum contract: the zero value is valid
// (unset → default), every registered ID is valid, and anything else is not.
func TestSearchProviderValid(t *testing.T) {
	tests := []struct {
		provider SearchProvider
		valid    bool
	}{
		{"", true},
		{SearchProviderTavily, true},
		{SearchProviderBrave, true},
		{SearchProviderSerpAPI, true},
		{"bogus", false},
		{"TAVILY", false},
	}
	for _, tt := range tests {
		if got := tt.provider.Valid(); got != tt.valid {
			t.Errorf("%q.Valid() = %v, want %v", tt.provider, got, tt.valid)
		}
	}
}

// TestSearchProviderIDsIsOrderedAndComplete guards the canonical ordered list
// the backend surfaces to the frontend (Tavily first) and that every entry is a
// valid provider with no duplicates.
func TestSearchProviderIDsIsOrderedAndComplete(t *testing.T) {
	ids := SearchProviderIDs()
	want := []SearchProvider{SearchProviderTavily, SearchProviderBrave, SearchProviderSerpAPI}
	if len(ids) != len(want) {
		t.Fatalf("SearchProviderIDs() = %v, want %v", ids, want)
	}
	seen := make(map[SearchProvider]bool, len(ids))
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("SearchProviderIDs()[%d] = %q, want %q", i, id, want[i])
		}
		if !id.Valid() || id == "" {
			t.Errorf("SearchProviderIDs()[%d] = %q is not a valid concrete provider", i, id)
		}
		if seen[id] {
			t.Errorf("SearchProviderIDs() contains duplicate %q", id)
		}
		seen[id] = true
	}
}

// TestSearchConfigValidate covers the save-boundary contract: an unset provider
// and zero max_results are valid (defaults apply); an unknown provider and an
// out-of-range max_results are rejected. Both failures are classified by
// IsSearchConfigError so the transport layer can map them to 400 in one check.
func TestSearchConfigValidate(t *testing.T) {
	tests := []struct {
		name     string
		cfg      SearchConfig
		wantErr  error
		classify bool
	}{
		{name: "empty config is valid", cfg: SearchConfig{}},
		{name: "default provider with no max is valid", cfg: SearchConfig{Provider: SearchProviderTavily}},
		{name: "explicit max results is valid", cfg: SearchConfig{Provider: SearchProviderBrave, MaxResults: 10}},
		{name: "max results at ceiling is valid", cfg: SearchConfig{Provider: SearchProviderSerpAPI, MaxResults: MaxSearchMaxResults}},
		{name: "unknown provider rejected", cfg: SearchConfig{Provider: "bogus"}, wantErr: ErrInvalidSearchProvider, classify: true},
		{name: "negative max results rejected", cfg: SearchConfig{Provider: SearchProviderTavily, MaxResults: -1}, wantErr: ErrSearchMaxResultsOutOfRange, classify: true},
		{name: "max results above ceiling rejected", cfg: SearchConfig{Provider: SearchProviderTavily, MaxResults: MaxSearchMaxResults + 1}, wantErr: ErrSearchMaxResultsOutOfRange, classify: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want errors.Is(%v)", err, tt.wantErr)
			}
			if got := IsSearchConfigError(err); got != tt.classify {
				t.Errorf("IsSearchConfigError(%v) = %v, want %v", err, got, tt.classify)
			}
		})
	}

	if IsSearchConfigError(nil) {
		t.Error("IsSearchConfigError(nil) = true, want false")
	}
}

// TestDefaultSearchConfig pins the default the lazy resolver falls back to when
// the operator has not chosen a provider.
func TestDefaultSearchConfig(t *testing.T) {
	got := DefaultSearchConfig()
	if got.Provider != SearchProviderTavily {
		t.Errorf("DefaultSearchConfig().Provider = %q, want %q", got.Provider, SearchProviderTavily)
	}
	if got.MaxResults != DefaultSearchMaxResults {
		t.Errorf("DefaultSearchConfig().MaxResults = %d, want %d", got.MaxResults, DefaultSearchMaxResults)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("DefaultSearchConfig().Validate() = %v, want nil", err)
	}
}
