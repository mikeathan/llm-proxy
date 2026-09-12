package guardrails

import (
	"context"
	"errors"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardrailEngine_GlobalGuardrails(t *testing.T) {
	tests := []struct {
		name         string
		blockSecrets bool
		args         string
		wantErr      bool
	}{
		{
			name:         "Allows benign input",
			blockSecrets: true,
			args:         `{"query": "best pizza in NYC"}`,
			wantErr:      false,
		},
		{
			name:         "Blocks OpenAI secret key",
			blockSecrets: true,
			args:         `{"message": "My key is sk-abc123abc123abc123abc123abc123abc123"}`,
			wantErr:      true,
		},
		{
			name:         "Blocks AWS access key",
			blockSecrets: true,
			args:         `{"command": "curl -X POST -d 'key=AKIA1234567890123456'"}`,
			wantErr:      true,
		},
		{
			name:         "Allows secrets when blocking is disabled",
			blockSecrets: false,
			args:         `{"query": "sk-123"}`,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := models.AgentGuardrailsConfig{}
			cfg.Global.BlockSecrets = tt.blockSecrets

			engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)

			call := proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      "test_tool",
					Arguments: tt.args,
				},
			}

			err := engine.ValidateToolCall(context.Background(), call, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateToolCall() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGuardrailEngine_SearchGuardrails(t *testing.T) {
	tests := []struct {
		name    string
		config  models.SearchGuardrailsConfig
		query   string
		wantErr bool
	}{
		{
			name: "Normal query works",
			config: models.SearchGuardrailsConfig{
				Enabled: true,
			},
			query:   `{"query": "weather"}`,
			wantErr: false,
		},
		{
			name: "Search disabled",
			config: models.SearchGuardrailsConfig{
				Enabled: false,
			},
			query:   `{"query": "weather"}`,
			wantErr: true,
		},
		{
			name: "Max length exceeded",
			config: models.SearchGuardrailsConfig{
				Enabled:     true,
				MaxQueryLen: 5,
			},
			query:   `{"query": "too long"}`,
			wantErr: true,
		},
		{
			name: "Blocked site detection",
			config: models.SearchGuardrailsConfig{
				Enabled:      true,
				BlockedSites: []string{"forbidden.com"},
			},
			query:   `{"query": "info from forbidden.com"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := models.AgentGuardrailsConfig{Search: tt.config}
			engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)

			call := proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      models.ToolInternetSearch,
					Arguments: tt.query,
				},
			}

			err := engine.ValidateToolCall(context.Background(), call, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateToolCall() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGuardrailEngine_CommunicationGuardrails(t *testing.T) {
	tests := []struct {
		name    string
		config  models.CommunicationGuardrailsConfig
		wantErr bool
		errText string
	}{
		{
			name: "Enabled works",
			config: models.CommunicationGuardrailsConfig{
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "Disabled fails",
			config: models.CommunicationGuardrailsConfig{
				Enabled: false,
			},
			wantErr: true,
		},
		{
			name: "Manual review flag",
			config: models.CommunicationGuardrailsConfig{
				Enabled:       true,
				RequireReview: true,
			},
			wantErr: true,
			errText: "manual approval required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := models.AgentGuardrailsConfig{Communication: tt.config}
			engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)

			call := proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name: models.ToolNotifyUser,
				},
			}

			err := engine.ValidateToolCall(context.Background(), call, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateToolCall() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errText != "" && err != nil && !strings.Contains(err.Error(), tt.errText) {
				t.Errorf("Expected error containing '%s', got '%v'", tt.errText, err)
			}
		})
	}
}

func TestDisabledToolNames(t *testing.T) {
	contains := func(names []string, want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}

	networkTools := []string{models.ToolNetworkFetch, models.ToolNetworkScan, models.ToolNetworkInfo}

	t.Run("all categories disabled by default", func(t *testing.T) {
		engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} }, storage.NewPathResolver("", "", ""), nil, nil)
		disabled := engine.DisabledToolNames("")
		for _, want := range append([]string{models.ToolNotifyUser, models.ToolInternetSearch}, networkTools...) {
			if !contains(disabled, want) {
				t.Errorf("expected %q to be disabled by default", want)
			}
		}
	})

	t.Run("enabled categories are preserved", func(t *testing.T) {
		cfg := models.AgentGuardrailsConfig{
			Communication: models.CommunicationGuardrailsConfig{Enabled: true},
			Search:        models.SearchGuardrailsConfig{Enabled: true},
			Network:       models.NetworkGuardrailsConfig{Enabled: true},
		}
		engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)
		if disabled := engine.DisabledToolNames(""); len(disabled) != 0 {
			t.Errorf("expected no disabled tools when all categories enabled, got %v", disabled)
		}
	})

	t.Run("only enabled category excluded from results", func(t *testing.T) {
		cfg := models.AgentGuardrailsConfig{
			Communication: models.CommunicationGuardrailsConfig{Enabled: true},
		}
		engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)
		disabled := engine.DisabledToolNames("")
		if contains(disabled, models.ToolNotifyUser) {
			t.Error("notify_user must not be disabled when communication is enabled")
		}
		if !contains(disabled, models.ToolInternetSearch) {
			t.Error("internet_search must be disabled when search is off")
		}
	})

	t.Run("workspace override enables a disabled category", func(t *testing.T) {
		base := models.AgentGuardrailsConfig{} // communication disabled globally
		readConfig := func(workspaceID string) (*models.WorkspaceConfig, error) {
			return &models.WorkspaceConfig{
				Guardrails: &models.AgentGuardrailsConfig{
					Communication: models.CommunicationGuardrailsConfig{Enabled: true},
				},
			}, nil
		}
		engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return base }, storage.NewPathResolver("", "", ""), nil, readConfig)
		disabled := engine.DisabledToolNames("ws-1")
		if contains(disabled, models.ToolNotifyUser) {
			t.Error("workspace override enabling communication must remove notify_user from disabled set")
		}
	})
}

func TestGuardrailEngine_FileSystem_DynamicWorkspace(t *testing.T) {
	wsDir := "workspaces" // Constant for test env

	cfg := models.AgentGuardrailsConfig{
		FileSystem: models.FileSystemGuardrailsConfig{
			Enabled:      true,
			ReadOnly:     false,
			AllowedPaths: []string{}, // No global paths
		},
	}

	resolver := storage.NewPathResolver(wsDir, wsDir, wsDir)
	engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return cfg
	}, resolver, nil, nil)

	ctx := context.Background()
	tests := []struct {
		name        string
		workspaceID string
		path        string
		wantErr     bool
	}{
		{
			name:        "Allow access to its own workspace",
			workspaceID: "test-ws",
			path:        filepath.Join(wsDir, "test-ws", "file.txt"),
			wantErr:     false,
		},
		{
			name:        "Reject access to another workspace (JAILBREAK)",
			workspaceID: "test-ws",
			path:        "../other-ws/file.txt",
			wantErr:     true,
		},
		{
			name:        "Protect system config in root",
			workspaceID: "test-ws",
			path:        models.ConfigFilename,
			wantErr:     true,
		},
		{
			name:        "Protect hidden internal files (relative)",
			workspaceID: "test-ws",
			path:        filepath.Join(models.InternalDirName, "metadata.log"),
			wantErr:     true,
		},
		{
			name:        "Protect hidden internal files (absolute/full)",
			workspaceID: "test-ws",
			path:        filepath.Join(wsDir, "test-ws", models.InternalDirName, "state.json"),
			wantErr:     true,
		},
		{
			name:        "Block access to config.yaml inside .internal",
			workspaceID: "test-ws",
			path:        filepath.Join(wsDir, "test-ws", models.InternalDirName, models.ConfigFilename),
			wantErr:     true,
		},
		{
			name:        "Block hidden dotfiles in root",
			workspaceID: "test-ws",
			path:        filepath.Join(wsDir, "test-ws", ".env"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      models.ToolFileRead,
					Arguments: `{"path": "` + strings.ReplaceAll(tt.path, `\`, `\\`) + `"}`,
				},
			}
			err := engine.ValidateToolCall(ctx, call, tt.workspaceID)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s: ValidateToolCall() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestGuardrailEngine_TerminalUsesBlockedFilenames(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{"blocks sandbox runtime dir", `{"command": "du -sh .sandbox"}`, true},
		{"blocks dotenv via terminal", `{"command": "cat .env"}`, true},
		{"blocks ssh key via terminal", `{"command": "cat .ssh/id_rsa"}`, true},
		{"allows workspace source glob", `{"command": "du -sh *"}`, false},
		{"allows git operations", `{"command": "git status"}`, false},
		{"allows npm install", `{"command": "npm install"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := models.AgentGuardrailsConfig{}
			cfg.Terminal.Enabled = true
			cfg.Terminal.AllowedCommands = []string{"du", "cat", "git", "npm"}
			cfg.FileSystem.BlockedFilenames = []string{".env", ".ssh", "id_rsa", "id_ed25519", ".pem"}

			engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)

			call := proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      models.ToolTerminalExecute,
					Arguments: tt.command,
				},
			}
			err := engine.ValidateToolCall(context.Background(), call, "test-ws")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateToolCall() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// enabledNetworkCfg returns a guardrail config whose network/search/
// communication categories are all enabled, so any schema/denial in the host
// gate tests is attributable to the host network switch, not the guardrail tier.
func enabledNetworkCfg() models.AgentGuardrailsConfig {
	cfg := models.AgentGuardrailsConfig{}
	cfg.Network.Enabled = true
	cfg.Network.AllowLanAccess = true
	cfg.Network.AllowInternetAccess = true
	cfg.Search.Enabled = true
	cfg.Communication.Enabled = true
	return cfg
}

// Host-level network gate (plan D1/R4): schema-hiding and hard denial when
// sandboxing.network is explicitly OFF. Overrides must never re-enable it.
func TestHostNetworkGate_DisabledToolNames(t *testing.T) {
	cfg := enabledNetworkCfg()
	callNames := func(disabled []string) map[string]bool {
		m := make(map[string]bool, len(disabled))
		for _, n := range disabled {
			m[n] = true
		}
		return m
	}

	t.Run("no host provider leaves tools visible", func(t *testing.T) {
		e := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)
		disabled := callNames(e.DisabledToolNames(""))
		for _, name := range hostNetworkGatedTools {
			if disabled[name] {
				t.Errorf("tool %q disabled without a host gate", name)
			}
		}
	})

	t.Run("host network ON keeps tools visible", func(t *testing.T) {
		e := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		disabled := callNames(e.DisabledToolNames(""))
		for _, name := range hostNetworkGatedTools {
			if disabled[name] {
				t.Errorf("tool %q disabled while host network allowed", name)
			}
		}
	})

	t.Run("host network OFF hides every network-gated tool", func(t *testing.T) {
		e := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return false })
		disabled := callNames(e.DisabledToolNames(""))
		for _, name := range hostNetworkGatedTools {
			if !disabled[name] {
				t.Errorf("host network off must schema-hide %q", name)
			}
		}
	})

	t.Run("override cannot keep a host-gated tool visible", func(t *testing.T) {
		e := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return false })
		e.MarkOverride("ws1", models.ToolNetworkFetch) // in-memory approval must NOT win
		disabled := callNames(e.DisabledToolNames("ws1"))
		if !disabled[models.ToolNetworkFetch] {
			t.Error("host gate must ignore in-memory overrides (plan R4)")
		}
	})
}

func TestHostNetworkGate_ValidateToolCallHardDenial(t *testing.T) {
	fetch := proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolNetworkFetch, Arguments: `{"url":"https://example.com"}`}}

	t.Run("host OFF denies fetch with the sentinel", func(t *testing.T) {
		e := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return enabledNetworkCfg() }, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return false })
		err := e.ValidateToolCall(context.Background(), fetch, "")
		if err == nil {
			t.Fatal("expected denial when host network is off")
		}
		if !errors.Is(err, ErrNetworkDisabled) {
			t.Errorf("expected ErrNetworkDisabled (errors.Is), got %v", err)
		}
	})

	t.Run("override cannot bypass the host gate", func(t *testing.T) {
		e := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return enabledNetworkCfg() }, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return false })
		e.MarkOverride("ws1", models.ToolNetworkFetch)
		err := e.ValidateToolCall(context.Background(), fetch, "ws1")
		if err == nil || !errors.Is(err, ErrNetworkDisabled) {
			t.Errorf("host gate must fire before the override fast path, got %v", err)
		}
	})

	t.Run("host ON validates through to category rules", func(t *testing.T) {
		e := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return enabledNetworkCfg() }, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		if err := e.ValidateToolCall(context.Background(), fetch, ""); err != nil {
			t.Errorf("fetch_url should pass category rules when host network is on: %v", err)
		}
	})

	t.Run("non-network tool unaffected by host gate", func(t *testing.T) {
		e := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return enabledNetworkCfg() }, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return false })
		terminal := proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolTerminalExecute, Arguments: `{"command":"ls"}`}}
		if err := e.ValidateToolCall(context.Background(), terminal, ""); err != nil && errors.Is(err, ErrNetworkDisabled) {
			t.Error("host network gate must not reject non-network tools")
		}
	})
}

// Run-scope grant model (plan §4.4): ResolveRunScope folds host (L0) + merged
// workspace guardrails (L1) + the automation grant (L2).
func TestGuardrailEngine_ResolveRunScope(t *testing.T) {
	wsInternet := func() models.AgentGuardrailsConfig {
		c := models.AgentGuardrailsConfig{}
		c.Network.Enabled = true
		c.Network.AllowLanAccess = true
		c.Network.AllowInternetAccess = true
		return c
	}
	wsOff := func() models.AgentGuardrailsConfig {
		c := models.AgentGuardrailsConfig{}
		c.Network.Enabled = false
		return c
	}

	tests := []struct {
		name  string
		host  *bool // nil = no host provider (allowed)
		cfg   func() models.AgentGuardrailsConfig
		grant models.NetworkScope
		want  models.NetworkScope
	}{
		{"host off beats internet grant", boolp(false), wsInternet, models.NetworkScopeInternet, models.NetworkScopeNone},
		{"grant none tightens internet ws", boolp(true), wsInternet, models.NetworkScopeNone, models.NetworkScopeNone},
		{"grant lan loosens offline ws", boolp(true), wsOff, models.NetworkScopeLan, models.NetworkScopeLan},
		{"inherit follows ws internet", boolp(true), wsInternet, models.NetworkScopeInherit, models.NetworkScopeInternet},
		{"inherit with ws off is none", boolp(true), wsOff, models.NetworkScopeInherit, models.NetworkScopeNone},
		{"no host provider defaults allowed", nil, wsInternet, models.NetworkScopeInherit, models.NetworkScopeInternet},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return tt.cfg() }, storage.NewPathResolver("", "", ""), nil, nil)
			if tt.host != nil {
				e.SetHostNetworkAllowed(func() bool { return *tt.host })
			}
			if got := e.ResolveRunScope("", tt.grant); got != tt.want {
				t.Errorf("ResolveRunScope(grant=%q) = %q, want %q", tt.grant, got, tt.want)
			}
		})
	}
}

func boolp(b bool) *bool { return &b }

func TestGuardrailEngine_ScopeAwareSchemaAndDenial(t *testing.T) {
	wsOffCfg := func() models.AgentGuardrailsConfig {
		c := models.AgentGuardrailsConfig{}
		c.Network.Enabled = false // workspace L1 network disabled
		c.Search.Enabled = true
		c.Communication.Enabled = true
		return c
	}

	t.Run("scope none hides every egress tool regardless of ws tier", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		disabled := e.DisabledToolNamesForScope("", models.NetworkScopeNone)
		for _, name := range hostNetworkGatedTools {
			if !containsString(disabled, name) {
				t.Errorf("scope none must hide %q", name)
			}
		}
	})

	t.Run("scope lan shows core network tools in an offline ws (L2 loosens)", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		disabled := mapSlice(e.DisabledToolNamesForScope("", models.NetworkScopeLan))
		for _, name := range []string{models.ToolNetworkFetch, models.ToolNetworkScan, models.ToolNetworkInfo} {
			if disabled[name] {
				t.Errorf("scope lan must expose %q in an offline workspace (L2 loosening)", name)
			}
		}
		// LAN is not internet: search and connector sends (internet-only egress)
		// must stay hidden even though their L1 tiers are enabled.
		for _, name := range []string{models.ToolInternetSearch, models.ToolNotifyUser} {
			if !disabled[name] {
				t.Errorf("scope lan must hide internet-only %q", name)
			}
		}
	})

	t.Run("scope lan hard-gates internet-only tools despite overrides", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		e.MarkOverride("ws1", models.ToolInternetSearch)
		e.MarkOverride("ws1", models.ToolNotifyUser)
		disabled := mapSlice(e.DisabledToolNamesForScope("ws1", models.NetworkScopeLan))
		if !disabled[models.ToolInternetSearch] || !disabled[models.ToolNotifyUser] {
			t.Errorf("lan-scope overrides must not re-expose internet-only tools: %+v", disabled)
		}
	})

	t.Run("ctx scope lan denies search and notify with the security sentinel", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		ctx := models.WithRunNetworkScope(context.Background(), models.NetworkScopeLan)
		for _, name := range []string{models.ToolInternetSearch, models.ToolNotifyUser} {
			err := e.ValidateToolCall(ctx, proxy.ToolCall{Function: proxy.FunctionCall{Name: name, Arguments: "{}"}}, "")
			if !errors.Is(err, ErrNetworkDisabled) {
				t.Errorf("scope lan must hard-deny %q (internet-only), got %v", name, err)
			}
		}
	})

	t.Run("scope internet re-exposes internet-only tools when their tier is on", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		disabled := mapSlice(e.DisabledToolNamesForScope("", models.NetworkScopeInternet))
		for _, name := range []string{models.ToolNetworkFetch, models.ToolInternetSearch, models.ToolNotifyUser} {
			if disabled[name] {
				t.Errorf("scope internet must expose %q when its L1 tier is enabled", name)
			}
		}
	})

	t.Run("ctx scope none denies even with host allowed", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		ctx := models.WithRunNetworkScope(context.Background(), models.NetworkScopeNone)
		fetch := proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolNetworkFetch, Arguments: `{"url":"https://example.com"}`}}
		err := e.ValidateToolCall(ctx, fetch, "")
		if !errors.Is(err, ErrNetworkDisabled) {
			t.Errorf("scope none must hard-deny network tools, got %v", err)
		}
	})

	t.Run("ctx scope internet validates through in an offline ws", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		ctx := models.WithRunNetworkScope(context.Background(), models.NetworkScopeInternet)
		fetch := proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolNetworkFetch, Arguments: `{"url":"https://example.com"}`}}
		if err := e.ValidateToolCall(ctx, fetch, ""); err != nil {
			t.Errorf("scope internet must allow fetch through to category checks in offline ws, got %v", err)
		}
	})

	t.Run("scope internet_only hides local-network tools but keeps internet egress", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		disabled := mapSlice(e.DisabledToolNamesForScope("", models.NetworkScopeInternetOnly))
		for _, name := range []string{models.ToolNetworkScan, models.ToolNetworkInfo} {
			if !disabled[name] {
				t.Errorf("scope internet_only must hide local-network tool %q", name)
			}
		}
		for _, name := range []string{models.ToolNetworkFetch, models.ToolInternetSearch, models.ToolNotifyUser} {
			if disabled[name] {
				t.Errorf("scope internet_only must keep %q visible", name)
			}
		}
	})

	t.Run("scope internet_only hard-gates local-network tools despite overrides", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		e.MarkOverride("ws1", models.ToolNetworkScan)
		e.MarkOverride("ws1", models.ToolNetworkInfo)
		disabled := mapSlice(e.DisabledToolNamesForScope("ws1", models.NetworkScopeInternetOnly))
		if !disabled[models.ToolNetworkScan] || !disabled[models.ToolNetworkInfo] {
			t.Errorf("internet_only overrides must not re-expose local-network tools: %+v", disabled)
		}
	})

	t.Run("ctx scope internet_only denies local-network tools with the security sentinel", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		ctx := models.WithRunNetworkScope(context.Background(), models.NetworkScopeInternetOnly)
		for _, name := range []string{models.ToolNetworkScan, models.ToolNetworkInfo} {
			err := e.ValidateToolCall(ctx, proxy.ToolCall{Function: proxy.FunctionCall{Name: name, Arguments: "{}"}}, "")
			if !errors.Is(err, ErrNetworkDisabled) {
				t.Errorf("scope internet_only must hard-deny %q (local network), got %v", name, err)
			}
		}
	})

	t.Run("ctx scope internet_only allows internet fetch through", func(t *testing.T) {
		e := NewGuardrailEngine(wsOffCfg, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		ctx := models.WithRunNetworkScope(context.Background(), models.NetworkScopeInternetOnly)
		fetch := proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolNetworkFetch, Arguments: `{"url":"https://example.com"}`}}
		if err := e.ValidateToolCall(ctx, fetch, ""); err != nil {
			t.Errorf("scope internet_only must allow internet fetch through, got %v", err)
		}
	})
}

func mapSlice(in []string) map[string]bool {
	m := make(map[string]bool, len(in))
	for _, s := range in {
		m[s] = true
	}
	return m
}

// Search availability gate: internet_search is schema-hidden when no provider is
// configured (the live availability predicate), in addition to the
// Search.Enabled policy gate. A nil predicate leaves the old behaviour intact.
func TestSearchAvailabilityGate(t *testing.T) {
	enabled := func() models.AgentGuardrailsConfig {
		c := models.AgentGuardrailsConfig{}
		c.Search.Enabled = true
		return c
	}
	disabled := func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{}
	}

	tests := []struct {
		name       string
		cfg        func() models.AgentGuardrailsConfig
		available  *bool // nil = no predicate installed
		wantHidden bool
	}{
		{name: "no predicate keeps enabled search visible", cfg: enabled, available: nil, wantHidden: false},
		{name: "available keeps enabled search visible", cfg: enabled, available: boolp(true), wantHidden: false},
		{name: "unavailable hides enabled search", cfg: enabled, available: boolp(false), wantHidden: true},
		{name: "policy-disabled stays hidden even when available", cfg: disabled, available: boolp(true), wantHidden: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewGuardrailEngine(tt.cfg, storage.NewPathResolver("", "", ""), nil, nil)
			if tt.available != nil {
				e.SetSearchAvailable(func() bool { return *tt.available })
			}
			got := mapSlice(e.DisabledToolNames(""))[models.ToolInternetSearch]
			if got != tt.wantHidden {
				t.Errorf("internet_search hidden = %v, want %v", got, tt.wantHidden)
			}
		})
	}

	t.Run("scope internet hides search when no provider is configured", func(t *testing.T) {
		e := NewGuardrailEngine(enabled, storage.NewPathResolver("", "", ""), nil, nil)
		e.SetHostNetworkAllowed(func() bool { return true })
		e.SetSearchAvailable(func() bool { return false })
		got := mapSlice(e.DisabledToolNamesForScope("", models.NetworkScopeInternet))[models.ToolInternetSearch]
		if !got {
			t.Error("unconfigured internet_search must be hidden even at internet scope")
		}
	})
}
