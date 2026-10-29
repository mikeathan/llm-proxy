package models

import (
	"context"
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

func TestOriginConnector(t *testing.T) {
	if got := OriginConnector(context.Background()); got != "" {
		t.Errorf("bare context origin = %q", got)
	}
	if got := OriginConnector(nil); got != "" { //nolint:staticcheck // nil-safety is the contract
		t.Errorf("nil context origin = %q", got)
	}
	if got := OriginConnector(WithOriginConnector(context.Background(), "telegram")); got != "telegram" {
		t.Errorf("origin = %q", got)
	}
}

func TestChatTimezone_Context(t *testing.T) {
	if got := ChatTimezone(context.Background()); got != "" {
		t.Errorf("unset = %q, want empty", got)
	}
	if got := ChatTimezone(WithChatTimezone(context.Background(), "Europe/London")); got != "Europe/London" {
		t.Errorf("got %q", got)
	}
	//nolint:staticcheck // nil context is part of the contract
	if got := ChatTimezone(nil); got != "" {
		t.Errorf("nil ctx = %q, want empty", got)
	}
}

// A message's model and profile choices ride the context; empty leaves the context alone.
func TestRequestScopedChoices(t *testing.T) {
	ctx := context.Background()
	if GetModelOverride(ctx) != "" || GetProfileRequest(ctx) != "" {
		t.Fatal("a bare context carries no choices")
	}
	if WithModelOverride(ctx, "") != ctx || WithProfileRequest(ctx, "") != ctx {
		t.Error("an empty choice must return the context unchanged")
	}
	ctx = WithProfileRequest(WithModelOverride(ctx, "qwen"), "research")
	if GetModelOverride(ctx) != "qwen" || GetProfileRequest(ctx) != "research" {
		t.Errorf("choices = %q / %q", GetModelOverride(ctx), GetProfileRequest(ctx))
	}
	if GetProfileRequest(nil) != "" || GetModelOverride(nil) != "" { //nolint:staticcheck // the helpers accept a nil context like GetRunID
		t.Error("a nil context carries no choices")
	}
}

// Every kind of run takes its id from one generator: unique, opaque, the same shape for chats and automations.
func TestNewRunID(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id := NewRunID()
		if len(id) != runIDHexLen || seen[id] {
			t.Fatalf("run id %q: want %d hex characters, unique", id, runIDHexLen)
		}
		seen[id] = true
	}
}

// A message can ask for helpers; the context says so, and a context that never asked says no.
func TestHelpersRequest(t *testing.T) {
	if HelpersRequested(context.Background()) || HelpersRequested(WithHelpersRequest(context.Background(), false)) {
		t.Error("a context that did not ask is not asking")
	}
	if !HelpersRequested(WithHelpersRequest(context.Background(), true)) {
		t.Error("a message that asked is asking")
	}
	if HelpersRequested(nil) { //nolint:staticcheck // the helpers accept a nil context like GetRunID
		t.Error("a nil context asks for nothing")
	}
}

func connectors(token string) CommunicationConfig {
	s := map[string]string{"chat_id": "1"}
	if token != "" {
		s[ConnectorSettingWebhookToken] = token
	}
	return CommunicationConfig{Connectors: map[string]ConnectorConfig{"tg": {Type: ConnectorTypeTelegram, Enabled: true, Settings: s}}}
}

// A read of the configuration says a webhook secret is set, not what it is: any caller of the admin API (a script token,
// a page on the same origin) would otherwise read a credential that lets it post as Telegram.
func TestMaskedWebhookSecrets_HidesTheValueButNotThatItExists(t *testing.T) {
	original := connectors("real-secret")
	masked := original.MaskedWebhookSecrets()
	if got := masked.Connectors["tg"].Settings[ConnectorSettingWebhookToken]; got != MaskedSecret {
		t.Errorf("masked = %q, want %q", got, MaskedSecret)
	}
	if masked.Connectors["tg"].Settings["chat_id"] != "1" {
		t.Error("other settings must be unchanged")
	}
	if original.Connectors["tg"].Settings[ConnectorSettingWebhookToken] != "real-secret" {
		t.Error("masking changed the stored configuration")
	}
	if _, set := connectors("").MaskedWebhookSecrets().Connectors["tg"].Settings[ConnectorSettingWebhookToken]; set {
		t.Error("a connector without a secret must not appear to have one")
	}
}

// The page sends back what it read: the mask means "keep what is stored", a new value replaces it, and a connector the
// page removed or renamed does not inherit another's secret.
func TestKeepStoredWebhookSecrets(t *testing.T) {
	stored := connectors("real-secret")

	next := connectors(MaskedSecret)
	next.KeepStoredWebhookSecrets(stored)
	if got := next.Connectors["tg"].Settings[ConnectorSettingWebhookToken]; got != "real-secret" {
		t.Errorf("a masked value must keep the stored secret, got %q", got)
	}

	replaced := connectors("new-secret")
	replaced.KeepStoredWebhookSecrets(stored)
	if got := replaced.Connectors["tg"].Settings[ConnectorSettingWebhookToken]; got != "new-secret" {
		t.Errorf("a new value must replace the stored one, got %q", got)
	}

	cleared := connectors("")
	cleared.KeepStoredWebhookSecrets(stored)
	if _, set := cleared.Connectors["tg"].Settings[ConnectorSettingWebhookToken]; set {
		t.Error("a connector sent without a secret has none: the page cleared it")
	}

	renamed := CommunicationConfig{Connectors: map[string]ConnectorConfig{"other": {Type: ConnectorTypeTelegram, Settings: map[string]string{ConnectorSettingWebhookToken: MaskedSecret}}}}
	renamed.KeepStoredWebhookSecrets(stored)
	if _, set := renamed.Connectors["other"].Settings[ConnectorSettingWebhookToken]; set {
		t.Error("a mask with nothing stored under that name must not become a secret (the mask itself would be the secret)")
	}
}

// The model server's environment holds whatever the operator put there, often a token for the model host. A read says a
// secret-looking variable is set, not what it is, and a write that sends the mask back keeps the stored value.
func TestMaskedEnvironment_HidesTheValuesOfSecretLookingNames(t *testing.T) {
	env := map[string]string{"HF_TOKEN": "hf_real", "OPENAI_API_KEY": "sk-real", "DB_PASSWORD": "pw", "MY_SECRET": "s", "CUDA_VISIBLE_DEVICES": "0", "OMP_NUM_THREADS": "8", "hf_token_lower": "x"}
	got := MaskedEnvironment(env)
	for _, name := range []string{"HF_TOKEN", "OPENAI_API_KEY", "DB_PASSWORD", "MY_SECRET", "hf_token_lower"} {
		if got[name] != MaskedSecret {
			t.Errorf("%s = %q, want the mask", name, got[name])
		}
	}
	if got["CUDA_VISIBLE_DEVICES"] != "0" || got["OMP_NUM_THREADS"] != "8" {
		t.Errorf("an ordinary variable must be shown as it is: %v", got)
	}
	if env["HF_TOKEN"] != "hf_real" {
		t.Error("masking changed the stored environment")
	}
	if MaskedEnvironment(nil) != nil {
		t.Error("no environment stays none")
	}
}

func TestKeepStoredEnvironmentSecrets(t *testing.T) {
	stored := map[string]string{"HF_TOKEN": "hf_real", "CUDA_VISIBLE_DEVICES": "0"}
	got := KeepStoredEnvironmentSecrets(map[string]string{"HF_TOKEN": MaskedSecret, "CUDA_VISIBLE_DEVICES": "1", "NEW_TOKEN": MaskedSecret}, stored)
	if got["HF_TOKEN"] != "hf_real" {
		t.Errorf("a masked value keeps the stored one, got %q", got["HF_TOKEN"])
	}
	if got["CUDA_VISIBLE_DEVICES"] != "1" {
		t.Error("an ordinary value is taken as sent")
	}
	if _, set := got["NEW_TOKEN"]; set {
		t.Error("a mask with nothing stored must not be saved as a value")
	}
	if replaced := KeepStoredEnvironmentSecrets(map[string]string{"HF_TOKEN": "new"}, stored); replaced["HF_TOKEN"] != "new" {
		t.Error("a typed value replaces the stored one")
	}
	if got := KeepStoredEnvironmentSecrets(nil, stored); got != nil {
		t.Errorf("nil stays nil (an update that omits the environment): %v", got)
	}
	if stored["HF_TOKEN"] != "hf_real" {
		t.Error("the stored map was changed")
	}
}
