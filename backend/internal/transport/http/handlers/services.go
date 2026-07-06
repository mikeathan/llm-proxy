package handlers

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/models"
	"time"
)

type RuntimeService interface {
	EnsureModel(context.Context, string) (llm.ModelInstance, error)
	RecordActivity(string)
	Sync()
	ListModels() []models.ModelConfig
	AddModel(models.ModelConfig) error
	UpdateModel(models.ModelConfig) error
	RemoveModel(string) error
	ActiveInfo() *llm.ActiveModelInfo
	ActiveLogs() string
	LastTokensPerSecond() (float64, time.Time)
	StopActive() error
	ClearLogs() error
	ModelHost() string
	SetModelHost(string)
	ListProviderModels(context.Context, string, string) ([]models.ProviderModelInfo, error)
	TestProviderConnection(ctx context.Context, providerName, apiKey, apiKeyName, baseURL string) error
	SelectModels() (string, string)
	ApplyModelOverrides(overrides map[string]models.ModelOverride)
}

type AdminService interface {
	// Tier 1: Infrastructure
	GetSystem() models.SystemConfig
	UpdateSystem(func(*models.SystemConfig)) error

	// Tier 2: Registry
	GetRegistry() models.RegistryData
	UpdateRegistry(func(*models.RegistryData)) error

	GetSettings() models.UserSettings
	UpdateSettings(func(*models.UserSettings)) error
	GetGuardrails() models.AgentGuardrailsConfig

	// Tier 3: Secrets
	Secrets() models.SecretsStore

	// Host Machine Isolated Settings
	HostSettings() models.HostSettings
	UpdateHostSettings(models.HostSettings) error

	// Helper accessors for UI / Tools
	WorkspacesDir() string
	GPUConfig() models.GPUConfig
	MetricsSnapshot() metrics.MetricsSnapshot
	ProcessLogger(workspaceID string) logging.Logger
	RootDir() string

	// Model/MCP management
	PersistModel(models.ModelConfig) error
	PersistReplaceModel(models.ModelConfig) error
	PersistDeleteModel(string) error
	DeleteProviderWithCleanup(provider string) error
	ResolveModelPath(string, string) string
	AddMCPServer(models.MCPServerConfig) error
	UpdateMCPServer(models.MCPServerConfig) error
	RemoveMCPServer(string) error
	ListMCPServers() []models.MCPServerConfig
	ListTemplates() ([]models.TemplateMetadata, error)
	GetTemplate(id string) (models.Template, error)
	Models() []models.ModelConfig
	Providers() map[string]models.ProviderItem
	SetGPUConfig(models.GPUConfig)
	SetWorkspacesDir(string)
	Environment() map[string]string
	ApplySystemUpdate(context.Context, models.SystemUpdatePayload) error
	ServiceCredentials() (id, secret string)
	ResetShell(workspaceID string) error
	ListShellSessions() []models.TerminalSessionView
	RunLoggingEnabled() bool
}

// WorkspaceService encapsulates workspace configuration, state, and file operations,
// replacing direct *persistence.WorkspaceManager access from transport handlers.
type WorkspaceService interface {
	GetConfig(workspaceID string) (*models.WorkspaceConfig, error)
	SaveConfig(workspaceID string, cfg *models.WorkspaceConfig) error
	MutateConfig(workspaceID string, fn func(*models.WorkspaceConfig)) error
	CreateWorkspace(id string, cfg *models.WorkspaceConfig, initialFiles map[string]string) error
	GetState(workspaceID string) (*models.AgentState, error)
	ListWorkspaces() ([]*models.Workspace, error)
	ListFiles(workspaceID string) ([]string, error)
	ReadTaskFile(workspaceID, filename string) (string, error)
	WriteTaskFile(workspaceID, filename, content string) error
	DeleteTaskFile(workspaceID, filename string) error
	DeleteWorkspace(workspaceID string) error
}

type workspaceService struct {
	mgr *persistence.WorkspaceManager
}

func NewWorkspaceService(mgr *persistence.WorkspaceManager) WorkspaceService {
	return &workspaceService{mgr: mgr}
}

func (s *workspaceService) GetConfig(workspaceID string) (*models.WorkspaceConfig, error) {
	return s.mgr.ReadConfig(workspaceID)
}

func (s *workspaceService) SaveConfig(workspaceID string, cfg *models.WorkspaceConfig) error {
	return s.mgr.WriteConfig(workspaceID, cfg)
}

func (s *workspaceService) MutateConfig(workspaceID string, fn func(*models.WorkspaceConfig)) error {
	lock, err := s.mgr.AcquireLock(workspaceID)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer s.mgr.ReleaseLock(lock)

	cfg, err := s.mgr.ReadConfig(workspaceID)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	fn(cfg)

	return s.mgr.WriteConfig(workspaceID, cfg)
}

func (s *workspaceService) CreateWorkspace(id string, cfg *models.WorkspaceConfig, initialFiles map[string]string) error {
	lock, err := s.mgr.AcquireLock(id)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer s.mgr.ReleaseLock(lock)

	for filename, content := range initialFiles {
		if err := s.mgr.WriteTaskFile(id, filename, content); err != nil {
			return fmt.Errorf("write %s: %w", filename, err)
		}
	}

	return s.mgr.WriteConfig(id, cfg)
}

func (s *workspaceService) GetState(workspaceID string) (*models.AgentState, error) {
	return s.mgr.ReadState(workspaceID)
}

func (s *workspaceService) ListWorkspaces() ([]*models.Workspace, error) {
	return s.mgr.ListWorkspaces()
}

func (s *workspaceService) ListFiles(workspaceID string) ([]string, error) {
	return s.mgr.ListFiles(workspaceID)
}

func (s *workspaceService) ReadTaskFile(workspaceID, filename string) (string, error) {
	return s.mgr.ReadTaskFile(workspaceID, filename)
}

func (s *workspaceService) WriteTaskFile(workspaceID, filename, content string) error {
	return s.mgr.WriteTaskFile(workspaceID, filename, content)
}

func (s *workspaceService) DeleteTaskFile(workspaceID, filename string) error {
	return s.mgr.DeleteTaskFile(workspaceID, filename)
}

func (s *workspaceService) DeleteWorkspace(workspaceID string) error {
	return s.mgr.DeleteWorkspace(workspaceID)
}

type AssistantService interface {
	NodeHerder() nodeherder.MCPService
	ClientProvider() proxy.LLMClientProvider
	Limiter() ratelimiter.Limiter
	Logger() logging.Logger
	SelectModels() (string, string)

	Engine() assistant.Engine
	ToolProvider() assistant.ToolProvider
	GuardrailEngine() *guardrails.GuardrailEngine
	GuardrailDecisionStore() *assistant.GuardrailDecisionStore
	Persistence() *persistence.WorkspaceManager
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	GetPlaybackClient(ctx context.Context, ref string) (proxy.Client, error)
	ModelConfig(modelName string) (models.ModelConfig, bool)
	Orchestrator() *orchestrator.Orchestrator
	ProcessLogger(workspaceID string) logging.Logger
	RootDir() string
	Events() assistant.EventPublisher
	MemoryStore() *memory.Store
	RecordDir() string
	RunLoggingEnabled() bool
}


