package assistant

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/sandbox"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func TestLocalToolRegistry_Discovery(t *testing.T) {
	r := NewLocalToolRegistry(nil, nil, nil, tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{}
	}), nil, nil)
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
	term := tools.NewTerminalTools(tools.TerminalToolsDeps{
		ConfigProvider: func(ctx context.Context) models.TerminalGuardrailsConfig {
			return models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"echo"},
				TimeoutSeconds:  10,
			}
		},
	})

	r := NewLocalToolRegistry(term, nil, nil, tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{}
	}), nil, nil)

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

// TestLocalToolRegistry_WriteErrorIsTruthful verifies write_file/append_file
// report failures as errors, never as their success string. Previously the
// handlers returned ("File written successfully", err), and
// executeSingleToolStep preferred the non-empty string — so failed writes were
// recorded as successes while the run still errored.
func TestLocalToolRegistry_WriteErrorIsTruthful(t *testing.T) {
	wsDir := t.TempDir()
	r := NewLocalToolRegistry(nil, nil, nil, tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{wsDir},
		}
	}), nil, nil)

	writeCall := func(path string) proxy.ToolCall {
		return proxy.ToolCall{Function: proxy.FunctionCall{
			Name:      models.ToolFileWrite,
			Arguments: fmt.Sprintf(`{"path": %q, "content": "x"}`, path),
		}}
	}

	t.Run("write to directory fails without success string", func(t *testing.T) {
		result, err := r.ExecuteTool(context.Background(), writeCall(wsDir))
		if err == nil {
			t.Fatal("expected write to a directory to fail")
		}
		if s, ok := result.(string); ok && s == "File written successfully" {
			t.Fatal("write failure must not be reported as the success string")
		}
	})

	t.Run("write success returns success string", func(t *testing.T) {
		result, err := r.ExecuteTool(context.Background(), writeCall("ok.txt"))
		if err != nil {
			t.Fatalf("expected write to succeed: %v", err)
		}
		if s, ok := result.(string); !ok || s != "File written successfully" {
			t.Errorf("expected success string, got %v", result)
		}
	})

	t.Run("append to missing file fails without success string", func(t *testing.T) {
		call := proxy.ToolCall{Function: proxy.FunctionCall{
			Name:      models.ToolFileAppend,
			Arguments: `{"path": "missing.txt", "content": "x"}`,
		}}
		result, err := r.ExecuteTool(context.Background(), call)
		if err == nil {
			t.Fatal("expected append to a missing file to fail")
		}
		if s, ok := result.(string); ok && s == "Content appended successfully" {
			t.Fatal("append failure must not be reported as the success string")
		}
	})
}

// TestLocalToolRegistry_SearchNilIsRecordAndContinue verifies the nil-safe
// search tool path: a registry wired without a search provider still executes
// internet_search and returns ErrSearchNotConfigured (record-and-continue)
// instead of panicking or raising an approval prompt.
func TestLocalToolRegistry_SearchNilIsRecordAndContinue(t *testing.T) {
	r := NewLocalToolRegistry(nil, nil, nil, tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{}
	}), nil, nil)

	call := proxy.ToolCall{Function: proxy.FunctionCall{
		Name:      models.ToolInternetSearch,
		Arguments: `{"query": "weather"}`,
	}}
	_, err := r.ExecuteTool(context.Background(), call)
	if !errors.Is(err, tools.ErrSearchNotConfigured) {
		t.Fatalf("ExecuteTool() err = %v, want ErrSearchNotConfigured", err)
	}
}

func TestInitializeAgentStack_Structure(t *testing.T) {
	// verify that InitializeAgentStack returns working objects
	provider, engine, guardrails := InitializeAgentStack(&mockAppContext{}, nil, AgentStackDeps{})

	if provider == nil || engine == nil || guardrails == nil {
		t.Fatal("InitializeAgentStack returned nil component")
	}

	_, ok := provider.(*MultiToolProvider)
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
	provider, _, _ := InitializeAgentStack(appCtx, nil, AgentStackDeps{})

	// 2. Access the MultiToolProvider
	multiProvider := provider.(*MultiToolProvider)

	// 3. Find the LocalToolRegistry (it's the first provider in InitializeAgentStack)
	localRegistry := multiProvider.Providers[0].(*LocalToolRegistry)

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

	resolver := storage.NewPathResolver(tmpRoot, wsDir, wsDir)
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
	provider, _, _ := InitializeAgentStack(appCtx, nil, AgentStackDeps{Persistence: manager})

	// Access the internal registry
	multiProvider := provider.(*MultiToolProvider)
	localRegistry := multiProvider.Providers[0].(*LocalToolRegistry)

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

func TestInitializeAgentStack_NetworkGuardrails(t *testing.T) {
	tmpRoot, _ := os.MkdirTemp("", "proxy-test-net-*")
	defer os.RemoveAll(tmpRoot)
	wsDir := filepath.Join(tmpRoot, "workspaces")
	resolver := storage.NewPathResolver(tmpRoot, wsDir, wsDir)
	manager := persistence.NewWorkspaceManager(resolver)

	wsID := "net-workspace"
	wsCfg := models.WorkspaceConfig{
		Guardrails: &models.AgentGuardrailsConfig{
			Network: models.NetworkGuardrailsConfig{
				Enabled:        true,
				AllowLanAccess: true,
			},
		},
	}
	manager.WriteConfig(wsID, &wsCfg)

	appCtx := &mockAppContextWithDirs{workspacesDir: wsDir}
	provider, _, _ := InitializeAgentStack(appCtx, nil, AgentStackDeps{Persistence: manager})
	multiProvider := provider.(*MultiToolProvider)
	localRegistry := multiProvider.Providers[0].(*LocalToolRegistry)

	t.Run("Should apply network overrides", func(t *testing.T) {
		ctx := models.WithWorkspaceID(context.Background(), wsID)
		netCfg := localRegistry.Network.Config(ctx)
		if !netCfg.AllowLanAccess {
			t.Error("Network override failed! Expected AllowLanAccess to be true")
		}
	})

	t.Run("Should use defaults for no workspace", func(t *testing.T) {
		ctx := context.Background()
		netCfg := localRegistry.Network.Config(ctx)
		if netCfg.AllowLanAccess {
			t.Error("Expected default network config (LanAccess=false), but got true")
		}
	})
}

type mockAppContext struct{}

func (m *mockAppContext) GetRegistry() models.RegistryData {
	return models.RegistryData{}
}
func (m *mockAppContext) GetGuardrails() models.AgentGuardrailsConfig {
	return models.AgentGuardrailsConfig{}
}
func (m *mockAppContext) Resolver() storage.Resolver {
	return storage.NewPathResolver("", "", "")
}
func (m *mockAppContext) Secrets() models.SecretsStore {
	return stubSearchSecrets{}
}
func (m *mockAppContext) MemoryStore() *memory.Store          { return nil }
func (m *mockAppContext) SetSandboxProvider(sandbox.Provider) {}

// HostSettings returns the LEGACY posture (sandboxing section zero → network
// undecided → allowed) so registry wiring tests behave like a pre-change
// install and the host gate stays inert.
func (m *mockAppContext) HostSettings() models.HostSettings {
	return models.HostSettings{}
}

type mockAppContextWithDirs struct {
	workspacesDir string
}

func (m *mockAppContextWithDirs) GetRegistry() models.RegistryData { return models.RegistryData{} }
func (m *mockAppContextWithDirs) GetGuardrails() models.AgentGuardrailsConfig {
	return models.AgentGuardrailsConfig{}
}
func (m *mockAppContextWithDirs) RootDir() string              { return "" }
func (m *mockAppContextWithDirs) Secrets() models.SecretsStore { return stubSearchSecrets{} }
func (m *mockAppContextWithDirs) Resolver() storage.Resolver {
	return storage.NewPathResolver("", m.workspacesDir, m.workspacesDir)
}
func (m *mockAppContextWithDirs) MemoryStore() *memory.Store          { return nil }
func (m *mockAppContextWithDirs) SetSandboxProvider(sandbox.Provider) {}

// HostSettings: legacy posture (network undecided → allowed); host gate inert.
func (m *mockAppContextWithDirs) HostSettings() models.HostSettings {
	return models.HostSettings{}
}

func TestSystemPrompt_IncludesMemoryNudge(t *testing.T) {
	fsTools := tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{}
	})

	t.Run("with memory tools", func(t *testing.T) {
		f, err := os.CreateTemp("", "registry-test-*.db")
		if err != nil {
			t.Fatal(err)
		}
		path := f.Name()
		f.Close()
		t.Cleanup(func() { os.Remove(path) })

		p, err := db.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { p.DB().Close() })

		store, err := memory.New(p)
		if err != nil {
			t.Fatal(err)
		}

		memTools := tools.NewMemoryToolProvider(store)
		r := NewLocalToolRegistry(nil, nil, nil, fsTools, nil, memTools)

		prompt, err := r.GetSystemPrompt()
		if err != nil {
			t.Fatalf("GetSystemPrompt failed: %v", err)
		}
		if !strings.Contains(prompt, "memory_update") {
			t.Error("expected proactive memory nudge in system prompt when memory tools are registered")
		}
	})

	t.Run("without memory tools", func(t *testing.T) {
		r := NewLocalToolRegistry(nil, nil, nil, fsTools, nil, nil)
		prompt, err := r.GetSystemPrompt()
		if err != nil {
			t.Fatalf("GetSystemPrompt failed: %v", err)
		}
		if strings.Contains(prompt, "Proactively use `memory_update`") {
			t.Error("unexpected memory nudge when no memory tools are registered")
		}
	})
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

// stubSearchSecrets is a minimal models.SecretsStore whose only meaningful
// method is GetSecret — all the search wiring reads. (The mocks package cannot
// be used here: it imports this package, which would be a test import cycle.)
type stubSearchSecrets struct{ key string }

func (s stubSearchSecrets) GetSecret(string, string) string                 { return s.key }
func (stubSearchSecrets) SetSecret(string, string, string) error            { return nil }
func (stubSearchSecrets) DeleteSecret(string, string) error                 { return nil }
func (stubSearchSecrets) MaskedSecret(string, string) string                { return "" }
func (stubSearchSecrets) GetProviderKeys(string) []models.APIKeyItem        { return nil }
func (stubSearchSecrets) SetProviderKeys(string, []models.APIKeyItem) error { return nil }
func (stubSearchSecrets) DeleteProviderKey(string, string) error            { return nil }
func (stubSearchSecrets) DeleteAllProviderKeys(string) error                { return nil }
func (stubSearchSecrets) MaskedProviderKeys(string) []models.APIKeyItem     { return nil }
func (stubSearchSecrets) GetResolvedProviderKey(string, string) (string, error) {
	return "", nil
}
func (stubSearchSecrets) GetResolvedProviderKeyInfo(string, string) (*models.ResolvedProviderKeyInfo, error) {
	return nil, nil
}
func (stubSearchSecrets) ResolveMaskedKey(string, string) (string, error) { return "", nil }

func TestConfigCache_EvictsOnMaxSize(t *testing.T) {
	cache := core.NewTTLCache[string, *workspaceConfigEntry](100, time.Hour, nil)

	const n = 101
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = "ws-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i/26))
	}
	for _, id := range ids {
		cache.Put(id, &workspaceConfigEntry{config: &models.WorkspaceConfig{}})
	}

	if got := cache.Len(); got != 100 {
		t.Errorf("expected cache bounded at 100 entries, got %d", got)
	}
}

func TestConfigCache_EvictsOnTTL(t *testing.T) {
	cache := core.NewTTLCache[string, *workspaceConfigEntry](100, time.Millisecond, nil)
	wsID := "ws-ttl"
	first := &workspaceConfigEntry{config: &models.WorkspaceConfig{}}
	cache.Put(wsID, first)

	time.Sleep(5 * time.Millisecond)

	reloaded, err := cache.Get(wsID, func() (*workspaceConfigEntry, error) {
		return &workspaceConfigEntry{config: &models.WorkspaceConfig{}}, nil
	})
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !cache.Contains(wsID) {
		t.Error("expected entry present after reload")
	}
	if reloaded == first {
		t.Error("expected a fresh entry after TTL expiry, got the same pointer (stale entry retained)")
	}
}

func TestGuardrailValidation_UsesCachedConfig(t *testing.T) {
	calls := 0
	cache := core.NewTTLCache[string, *workspaceConfigEntry](100, time.Hour, nil)
	for i := 0; i < 3; i++ {
		cache.Get("ws-cached", func() (*workspaceConfigEntry, error) {
			calls++
			return &workspaceConfigEntry{config: &models.WorkspaceConfig{}}, nil
		})
	}
	if calls != 1 {
		t.Errorf("cache should serve from memory on subsequent hits: got %d loads", calls)
	}
}

// fakeSearchAppCtx is the minimal appCtx the search wiring needs.
type fakeSearchAppCtx struct {
	reg models.RegistryData
	sec models.SecretsStore
}

func (f fakeSearchAppCtx) GetRegistry() models.RegistryData { return f.reg }
func (f fakeSearchAppCtx) Secrets() models.SecretsStore     { return f.sec }

func TestSearchTarget(t *testing.T) {
	secrets := func(key string) models.SecretsStore {
		return stubSearchSecrets{key: key}
	}
	reg := func(provider models.SearchProvider, max int) func() models.RegistryData {
		return func() models.RegistryData {
			return models.RegistryData{Search: models.SearchConfig{Provider: provider, MaxResults: max}}
		}
	}

	tests := []struct {
		name     string
		registry func() models.RegistryData
		secrets  models.SecretsStore
		wantOK   bool
		wantProv models.SearchProvider
		wantMax  int
	}{
		{name: "unset provider defaults to tavily with key", registry: reg("", 0), secrets: secrets("k"), wantOK: true, wantProv: models.SearchProviderTavily},
		{name: "unknown provider is unavailable", registry: reg("bogus", 5), secrets: secrets("k"), wantOK: false},
		{name: "missing key is unavailable", registry: reg(models.SearchProviderBrave, 5), secrets: secrets(""), wantOK: false},
		{name: "selected provider with key is available", registry: reg(models.SearchProviderBrave, 9), secrets: secrets("k"), wantOK: true, wantProv: models.SearchProviderBrave, wantMax: 9},
		{name: "nil registry falls back to the default", registry: nil, secrets: secrets("k"), wantOK: true, wantProv: models.SearchProviderTavily, wantMax: models.DefaultSearchMaxResults},
		{name: "nil secrets is unavailable for a key-requiring provider", registry: reg(models.SearchProviderTavily, 5), secrets: nil, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel := searchTarget(tt.registry, tt.secrets)
			if sel.ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", sel.ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if sel.provider != tt.wantProv {
				t.Errorf("provider = %q, want %q", sel.provider, tt.wantProv)
			}
			if sel.max != tt.wantMax {
				t.Errorf("max = %d, want %d", sel.max, tt.wantMax)
			}
		})
	}
}

func TestInitSearchTools_AvailabilityGatesResidualCalls(t *testing.T) {
	network := tools.NewNetworkTools(func(context.Context) models.NetworkGuardrailsConfig {
		return models.NetworkGuardrailsConfig{}
	}, nil)

	withKey := fakeSearchAppCtx{
		reg: models.RegistryData{Search: models.SearchConfig{Provider: models.SearchProviderTavily, MaxResults: 3}},
		sec: stubSearchSecrets{key: "k"},
	}
	internet, available := initSearchTools(withKey, network)
	if internet == nil {
		t.Fatal("initSearchTools returned a nil tool")
	}
	if !available() {
		t.Error("availability predicate = false, want true with a stored key")
	}

	withoutKey := fakeSearchAppCtx{
		reg: withKey.reg,
		sec: stubSearchSecrets{},
	}
	internet, available = initSearchTools(withoutKey, network)
	if available() {
		t.Error("availability predicate = true, want false without a key")
	}
	// A residual call (schema-hide is the only gate) must reach the tool and
	// return a clear error, never panic or hit the network.
	if _, err := internet.Search(context.Background(), "query"); !errors.Is(err, tools.ErrSearchNotConfigured) {
		t.Fatalf("residual Search() err = %v, want ErrSearchNotConfigured", err)
	}
}
