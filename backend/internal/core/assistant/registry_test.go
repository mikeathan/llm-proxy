package assistant_test

import (
	"context"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalToolRegistry_Discovery(t *testing.T) {
	r := assistant.NewLocalToolRegistry(nil, nil, nil, tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig { return models.FileSystemGuardrailsConfig{} }))
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
	term := tools.NewTerminalTools(func(ctx context.Context) models.TerminalGuardrailsConfig {
		return models.TerminalGuardrailsConfig{
			Enabled:         true,
			AllowedCommands: []string{"echo"},
			TimeoutSeconds:  10,
		}
	})

	r := assistant.NewLocalToolRegistry(term, nil, nil, tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig { return models.FileSystemGuardrailsConfig{} }))

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

func TestInitializeAgentStack_FileSystemIsolation(t *testing.T) {
	workspacesDir := "/tmp/proxy-workspaces"
	appCtx := &mockAppContextWithDirs{
		workspacesDir: workspacesDir,
	}

	// 1. Initialize the stack
	provider, _, _ := assistant.InitializeAgentStack(appCtx, nil, nil, nil)

	// 2. Access the MultiToolProvider 
	multiProvider := provider.(*assistant.MultiToolProvider)
	
	// 3. Find the LocalToolRegistry (it's the first provider in InitializeAgentStack)
	localRegistry := multiProvider.Providers[0].(*assistant.LocalToolRegistry)
	
	// 4. Inspect its FileSystem configuration via the public method
	fsCfg := localRegistry.FileSystem.Config(context.Background())

	// VERIFY: The root workspaces directory MUST NOT be in allowed_paths.
	for _, path := range fsCfg.AllowedPaths {
		if path == workspacesDir {
			t.Errorf("SECURITY FAILURE: root workspaces directory %q was found in allowed_paths.", workspacesDir)
		}
	}
}

func TestInitializeAgentStack_ContextualSecurity(t *testing.T) {
	// 1. Setup temporary test environment
	tmpRoot, err := os.MkdirTemp("", "proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpRoot)

	wsDir := filepath.Join(tmpRoot, "workspaces")
	os.MkdirAll(wsDir, 0755)

	resolver := storage.NewPathResolver(wsDir)
	manager := persistence.NewWorkspaceManager(resolver)

	// 2. Create a workspace with specific overrides
	wsID := "test-vault"
	customTimeout := 123
	
	// Prepare a config with a custom timeout and a custom allowed path
	wsCfg := models.WorkspaceConfig{
		Guardrails: &models.AgentGuardrailsConfig{
			Terminal: models.TerminalGuardrailsConfig{
				Enabled:        true,
				TimeoutSeconds: customTimeout,
			},
			FileSystem: models.FileSystemGuardrailsConfig{
				Enabled:      true,
				AllowedPaths: []string{"/tmp/custom-authorized-path"},
			},
		},
	}
	
	// Write the config to the workspace's .internal directory
	if err := manager.WriteConfig(wsID, &wsCfg); err != nil {
		t.Fatalf("failed to write workspace config: %v", err)
	}

	// 3. Initialize the Agent Stack
	appCtx := &mockAppContextWithDirs{workspacesDir: wsDir}
	provider, _, _ := assistant.InitializeAgentStack(appCtx, manager, nil, nil)
	
	// Access the internal registry
	multiProvider := provider.(*assistant.MultiToolProvider)
	localRegistry := multiProvider.Providers[0].(*assistant.LocalToolRegistry)

	// ========================================================================
	// Scenario A: Default context (no workspace ID)
	// ========================================================================
	t.Run("Default_Context_Should_Use_Global_Manifests", func(t *testing.T) {
		ctx := context.Background()
		
		termCfg := localRegistry.Terminal.Config(ctx)
		if termCfg.TimeoutSeconds == customTimeout {
			t.Errorf("expected default timeout, but got workspace override %d", termCfg.TimeoutSeconds)
		}
		
		fsCfg := localRegistry.FileSystem.Config(ctx)
		for _, p := range fsCfg.AllowedPaths {
			if p == "/tmp/custom-authorized-path" {
				t.Errorf("expected global allowed paths, but found workspace override path")
			}
		}
	})

	// ========================================================================
	// Scenario B: Context with workspace ID
	// ========================================================================
	t.Run("Workspace_Context_Should_Apply_Overrides", func(t *testing.T) {
		// Inject workspace ID into context
		ctx := models.WithWorkspaceID(context.Background(), wsID)
		
		// 1. Verify Terminal Timeout Override
		termCfg := localRegistry.Terminal.Config(ctx)
		if termCfg.TimeoutSeconds != customTimeout {
			t.Errorf("Terminal override failed! Expected timeout %d, got %d", customTimeout, termCfg.TimeoutSeconds)
		}

		// 2. Verify FileSystem AllowedPaths Merging + Jaling
		fsCfg := localRegistry.FileSystem.Config(ctx)
		
		foundCustom := false
		foundJail := false
		expectedJail := resolver.WorkspaceDir(wsID)
		
		for _, p := range fsCfg.AllowedPaths {
			if p == "/tmp/custom-authorized-path" {
				foundCustom = true
			}
			if p == expectedJail {
				foundJail = true
			}
		}
		
		if !foundCustom {
			t.Error("FileSystem override failed! Custom allowed path not found")
		}
		if !foundJail {
			t.Errorf("Dynamic jailing failed! Expected jail path %s not found in allowed_paths", expectedJail)
		}
	})
}

type mockAppContext struct{}

func (m *mockAppContext) GetSystem() models.SystemConfig {
	return models.SystemConfig{}
}
func (m *mockAppContext) GetRegistry() models.RegistryData {
	return models.RegistryData{}
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

type mockAppContextWithDirs struct {
	workspacesDir string
}
func (m *mockAppContextWithDirs) GetSystem() models.SystemConfig { return models.SystemConfig{} }
func (m *mockAppContextWithDirs) GetRegistry() models.RegistryData { return models.RegistryData{} }
func (m *mockAppContextWithDirs) RootDir() string               { return "" }
func (m *mockAppContextWithDirs) Secrets() models.SecretsStore  { return &mocks.MockSecretsStore{} }
func (m *mockAppContextWithDirs) WorkspacesDir() string         { return m.workspacesDir }

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
