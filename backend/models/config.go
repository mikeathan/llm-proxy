package models

import (
	"context"
)

type contextKey string

const WorkspaceIDKey contextKey = "workspace_id"

// GetWorkspaceID retrieves the workspace ID from the context.
func GetWorkspaceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(WorkspaceIDKey).(string); ok {
		return id
	}
	return ""
}

// WithWorkspaceID injects the workspace ID into the context.
func WithWorkspaceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, WorkspaceIDKey, id)
}

type Config struct {
	Server        ServerConfig            `json:"server"`
	Providers     map[string]ProviderItem `json:"providers"`
	Models        []ModelConfig           `json:"models"`
	Agents        []AgentDefinition       `json:"agents,omitempty"`
	WorkspacesDir string                  `json:"workspaces_dir,omitempty"`
	Metrics       MetricsConfig           `json:"metrics,omitempty"`
	MCPServers    []MCPServerConfig       `json:"mcp_servers,omitempty"`
	Guardrails    *AgentGuardrailsConfig  `json:"guardrails,omitempty"`
	Communication CommunicationConfig     `json:"communication,omitempty"`
	Search        SearchConfig            `json:"search,omitempty"`
}

type AgentGuardrailsConfig struct {
	Global        GlobalGuardrailsConfig        `json:"global" yaml:"global"`
	Terminal      TerminalGuardrailsConfig      `json:"terminal" yaml:"terminal"`
	Search        SearchGuardrailsConfig        `json:"search" yaml:"search"`
	Communication CommunicationGuardrailsConfig `json:"communication" yaml:"communication"`
	FileSystem    FileSystemGuardrailsConfig    `json:"filesystem" yaml:"filesystem"`
	Network       NetworkGuardrailsConfig       `json:"network" yaml:"network"`
}

type GlobalGuardrailsConfig struct {
	BlockSecrets bool     `json:"block_secrets" yaml:"blocksecrets"`
	UserBlocked  []string `json:"user_blocked_patterns" yaml:"userblocked"`
}

type SearchGuardrailsConfig struct {
	Enabled      bool     `json:"enabled" yaml:"enabled"`
	MaxQueryLen  int      `json:"max_query_len" yaml:"maxquerylen"`
	BlockedSites []string `json:"blocked_sites" yaml:"blockedsites"`
}

type CommunicationGuardrailsConfig struct {
	Enabled       bool `json:"enabled" yaml:"enabled"`
	RequireReview bool `json:"require_review" yaml:"requirereview"`
	MaxMessages   int  `json:"max_messages_per_task" yaml:"maxmessages"`
}

type FileSystemGuardrailsConfig struct {
	Enabled           bool     `json:"enabled" yaml:"enabled"`
	AllowedPaths      []string `json:"allowed_paths" yaml:"allowedpaths"`
	ReadOnly          bool     `json:"read_only" yaml:"readonly"`
	MaxFileSizeKB     int      `json:"max_file_size_kb" yaml:"maxfilesizekb"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty" yaml:"allowedextensions"`
	BlockedFilenames  []string `json:"blocked_filenames,omitempty" yaml:"blockedfilenames"`
}

type TerminalGuardrailsConfig struct {
	Enabled         bool     `json:"enabled" yaml:"enabled"`
	AllowedCommands []string `json:"allowed_commands" yaml:"allowedcommands"`
	BlockedPatterns []string `json:"blocked_patterns,omitempty" yaml:"blockedpatterns"`
	TimeoutSeconds  int      `json:"timeout_seconds" yaml:"timeoutseconds"`
	MaxOutputSize   int      `json:"max_output_size_chars" yaml:"maxoutputsize"`
}

type NetworkGuardrailsConfig struct {
	Enabled             bool     `json:"enabled" yaml:"enabled"`
	AllowLanAccess      bool     `json:"allow_lan_access" yaml:"allowlanaccess"`
	AllowInternetAccess bool     `json:"allow_internet_access" yaml:"allowinternetaccess"`
	BlockedDomains      []string `json:"blocked_domains,omitempty" yaml:"blockeddomains"`
	BlockedIPs          []string `json:"blocked_ips,omitempty" yaml:"blockedips"`
	MaxFetchSizeKB      int      `json:"max_fetch_size_kb" yaml:"maxfetchsizekb"`
	TimeoutSeconds      int      `json:"timeout_seconds" yaml:"timeoutseconds"`
}

func (c TerminalGuardrailsConfig) IsActive() bool {
	return c.Enabled || len(c.AllowedCommands) > 0
}

func (c FileSystemGuardrailsConfig) IsActive() bool {
	return c.Enabled || len(c.AllowedPaths) > 0
}

func (c SearchGuardrailsConfig) IsActive() bool {
	return c.Enabled || c.MaxQueryLen > 0
}

func (c CommunicationGuardrailsConfig) IsActive() bool {
	return c.Enabled || c.MaxMessages > 0
}

func (c GlobalGuardrailsConfig) IsActive() bool {
	return c.BlockSecrets || len(c.UserBlocked) > 0
}

func (c NetworkGuardrailsConfig) IsActive() bool {
	return c.Enabled || c.AllowLanAccess || c.AllowInternetAccess
}

func (c *AgentGuardrailsConfig) MergeWith(other *AgentGuardrailsConfig) {
	if other == nil {
		return
	}
	if other.Terminal.IsActive() {
		c.Terminal = other.Terminal
	}
	if other.FileSystem.IsActive() {
		c.FileSystem = other.FileSystem
	}
	if other.Search.IsActive() {
		c.Search = other.Search
	}
	if other.Communication.IsActive() {
		c.Communication = other.Communication
	}
	if other.Global.IsActive() {
		c.Global = other.Global
	}
	if other.Network.IsActive() {
		c.Network = other.Network
	}
}

type CommunicationConfig struct {
	Telegram struct {
		Enabled bool   `json:"enabled"`
		ChatID  string `json:"chat_id"`
	} `json:"telegram"`
}

type SearchConfig struct {
}

type AgentDefinition struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ProviderID   string   `json:"provider_id"`
	ModelID      string   `json:"model_id"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
}

type APIKeyItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

type ProviderItem struct {
	Type              string            `json:"type"`
	BaseURL           string            `json:"base_url,omitempty"`
	ProjectID         string            `json:"project_id,omitempty"`
	Region            string            `json:"region,omitempty"`
	LlamaServerBinary string            `json:"llama_server_binary,omitempty"`
	ModelDir          string            `json:"model_dir,omitempty"`
	DefaultArgs       []string          `json:"default_args,omitempty"`
	Environment       map[string]string `json:"environment,omitempty"`
}

type MCPServerConfig struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	TLSCACert string `json:"tls_ca_cert,omitempty"` 
}

type ServerConfig struct {
	Bind              string            `json:"bind"`
	ModelHost         string            `json:"model_host"`
	IdleTimeoutSecs   int               `json:"idle_timeout_seconds"`
	LlamaServerBinary string            `json:"llama_server_binary"`
	DefaultArgs       []string          `json:"default_args"`
	Environment       map[string]string `json:"environment"`
	PrimaryModel      string            `json:"primary_model,omitempty"`
	FallbackModel     string            `json:"fallback_model,omitempty"`
}

type ModelConfig struct {
	Name           string            `json:"name"`
	Provider       string            `json:"provider"`
	Filename       string            `json:"filename,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Port           int               `json:"port,omitempty"`
	Path           string            `json:"-"`
	Environment    map[string]string `json:"environment,omitempty"`
	ProviderConfig ProviderConfig    `json:"provider_config,omitempty"`
}

type ProviderConfig struct {
	APIKey     string `json:"-"`
	APIKeyName string `json:"api_key_name,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	Region     string `json:"region,omitempty"`
}

type MetricsConfig struct {
	GPU GPUConfig `json:"gpu"`
}

type GPUConfig struct {
	Provider  string `json:"provider,omitempty"`
	Binary    string `json:"binary,omitempty"`
	Index     int    `json:"index,omitempty"`
	SysfsPath string `json:"sysfs_path,omitempty"`
}
