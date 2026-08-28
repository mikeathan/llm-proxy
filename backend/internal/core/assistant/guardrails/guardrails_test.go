package guardrails

import (
	"context"
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
