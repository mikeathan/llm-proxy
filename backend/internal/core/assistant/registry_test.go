package assistant_test

import (
	"context"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
	"testing"
)

func TestLocalToolRegistry_Discovery(t *testing.T) {
	r := assistant.NewLocalToolRegistry(nil, nil, nil, tools.NewFileSystemTools(func() models.FileSystemGuardrailsConfig { return models.FileSystemGuardrailsConfig{} }))
	toolsList, err := r.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	// Should have terminal, communication, search, and fs tools
	if len(toolsList) < 6 {
		t.Errorf("expected at least 6 tool definitions, got %d", len(toolsList))
	}
}

func TestLocalToolRegistry_TerminalExecution(t *testing.T) {
	term := tools.NewTerminalTools(func() models.TerminalGuardrailsConfig {
		return models.TerminalGuardrailsConfig{
			Enabled:         true,
			AllowedCommands: []string{"echo"},
		}
	})

	r := assistant.NewLocalToolRegistry(term, nil, nil, tools.NewFileSystemTools(func() models.FileSystemGuardrailsConfig { return models.FileSystemGuardrailsConfig{} }))

	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name:      models.ToolTerminalExecute,
			Arguments: `{"command": "echo hello"}`,
		},
	}

	result, err := r.ExecuteTool(context.Background(), call)
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestInitializeAgentStack_Structure(t *testing.T) {
	// verify that InitializeAgentStack returns working objects
	provider, engine, guardrails := assistant.InitializeAgentStack(&mockAppContext{}, nil, nil, nil)

	if provider == nil || engine == nil || guardrails == nil {
		t.Fatal("InitializeAgentStack returned nil component")
	}

	_, ok := provider.(*assistant.MultiToolProvider)
	if !ok {
		t.Error("provider is not a MultiToolProvider")
	}
}

type mockAppContext struct{}

func (m *mockAppContext) GetSystem() models.SystemConfig {
	return models.SystemConfig{}
}
func (m *mockAppContext) RootDir() string {
	return ""
}
func (m *mockAppContext) Secrets() models.SecretsStore {
	return &mocks.MockSecretsStore{}
}
func (m *mockAppContext) WorkspacesDir() string {
	return ""
}

func TestFileSystem_IsSecurePath(t *testing.T) {
	allowed := []string{"/tmp/test_workspace"}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "Allowed subfolder",
			path:    "/tmp/test_workspace/reports/scan.txt",
			wantErr: false,
		},
		{
			name:    "Forbidden path",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "Traversal attempt",
			path:    "/tmp/test_workspace/../passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tools.IsSecurePath(tt.path, allowed)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsSecurePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
