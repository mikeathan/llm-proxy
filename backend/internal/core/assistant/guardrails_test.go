package assistant

import (
	"context"
	"llm-proxy/internal/core/proxy"
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
			
			engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, "")
			
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
			engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, "")
			
			call := proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      "internet_search",
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
			engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, "")
			
			call := proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name: "notify_user",
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

func TestGuardrailEngine_FileSystem_DynamicWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "data", "workspaces")

	cfg := models.AgentGuardrailsConfig{
		FileSystem: models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{}, // No global paths
		},
	}

	// Initialize with the data/workspaces root
	engine := NewGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, wsDir)

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
			path:        filepath.Join(wsDir, "other-ws", "file.txt"),
			wantErr:     true,
		},
		{
			name:        "Reject outside access",
			workspaceID: "test-ws",
			path:        filepath.Join(tmpDir, "outside.txt"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      "read_file",
					Arguments: `{"path": "` + strings.ReplaceAll(tt.path, `\`, `\\`) + `"}`,
				},
			}
			err := engine.ValidateToolCall(context.Background(), call, tt.workspaceID)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s: ValidateToolCall() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}
