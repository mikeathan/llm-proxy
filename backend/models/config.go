// Package models defines all shared types for the llm-proxy system:
// model configs, provider definitions, guardrail rules, LLM messages,
// workspace state, and the resource-aware orchestration types.

package models

import (
	"context"
)

type contextKey string

const (
	WorkspaceIDKey       contextKey = "workspace_id"
	GuardrailApprovedKey contextKey = "guardrail_approved"
	TaskNameKey          contextKey = "task_name"
	RunIDKey             contextKey = "run_id"
)

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

// GetGuardrailApproved returns true if the context carries a guardrail-
// approval marker, signalling that the caller has already validated the
// tool call through the user-facing guardrail decision flow.
func GetGuardrailApproved(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(GuardrailApprovedKey).(bool)
	return v
}

// WithGuardrailApproved marks the context so downstream validators know
// the tool call was already approved by the user.
func WithGuardrailApproved(ctx context.Context) context.Context {
	return context.WithValue(ctx, GuardrailApprovedKey, true)
}

// WithTaskName injects the automation task name into the context.
// The recorder uses this to organise recordings into model/task subdirectories.
func WithTaskName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, TaskNameKey, name)
}

// GetTaskName retrieves the automation task name from the context.
func GetTaskName(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if name, ok := ctx.Value(TaskNameKey).(string); ok {
		return name
	}
	return ""
}

// WithRunID injects a unique execution run ID into the context.
// The recorder uses this to create a new file per execution run.
func WithRunID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, RunIDKey, id)
}

// GetRunID retrieves the execution run ID from the context.
func GetRunID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(RunIDKey).(string); ok {
		return id
	}
	return ""
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
	Enabled                   bool     `json:"enabled" yaml:"enabled"`
	AllowedCommands           []string `json:"allowed_commands" yaml:"allowedcommands"`
	AllowedEnvVars            []string `json:"allowed_env_vars,omitempty" yaml:"allowedenvvars"`
	BlockedPatterns           []string `json:"blocked_patterns,omitempty" yaml:"blockedpatterns"`
	PathExtensions            []string `json:"path_extensions,omitempty" yaml:"path_extensions"`
	AllowedExternalPaths      []string `json:"allowed_external_paths,omitempty" yaml:"allowedexternalpaths"`
	TimeoutSeconds            int      `json:"timeout_seconds" yaml:"timeoutseconds"`
	SessionIdleTimeoutSeconds int      `json:"session_idle_timeout_seconds" yaml:"sessionidletimeoutseconds"`
	MaxOutputSize             int      `json:"max_output_size_chars" yaml:"maxoutputsize"`
	DefaultShell              string   `json:"default_shell,omitempty" yaml:"defaultshell"`
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

func (c TerminalGuardrailsConfig) HasExternalAccess() bool {
	return len(c.AllowedExternalPaths) > 0
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

	// Helper to merge unique strings into a slice while ensuring a new slice is created
	mergeSlices := func(base []string, overrides []string) []string {
		res := make([]string, 0, len(base)+len(overrides))
		seen := make(map[string]bool)
		for _, s := range base {
			if !seen[s] {
				res = append(res, s)
				seen[s] = true
			}
		}
		for _, s := range overrides {
			if !seen[s] {
				res = append(res, s)
				seen[s] = true
			}
		}
		return res
	}

	// 1. Global
	if other.Global.BlockSecrets {
		c.Global.BlockSecrets = true
	}
	c.Global.UserBlocked = mergeSlices(c.Global.UserBlocked, other.Global.UserBlocked)

	// 2. Terminal
	if other.Terminal.Enabled {
		c.Terminal.Enabled = true
	}
	if other.Terminal.TimeoutSeconds > 0 {
		c.Terminal.TimeoutSeconds = other.Terminal.TimeoutSeconds
	}
	// We allow 0 to override manifest defaults (0 = disabled/infinite)
	c.Terminal.SessionIdleTimeoutSeconds = other.Terminal.SessionIdleTimeoutSeconds
	if other.Terminal.MaxOutputSize > 0 {
		c.Terminal.MaxOutputSize = other.Terminal.MaxOutputSize
	}
	c.Terminal.AllowedCommands = mergeSlices(c.Terminal.AllowedCommands, other.Terminal.AllowedCommands)
	c.Terminal.AllowedEnvVars = mergeSlices(c.Terminal.AllowedEnvVars, other.Terminal.AllowedEnvVars)
	c.Terminal.BlockedPatterns = mergeSlices(c.Terminal.BlockedPatterns, other.Terminal.BlockedPatterns)
	c.Terminal.PathExtensions = mergeSlices(c.Terminal.PathExtensions, other.Terminal.PathExtensions)
	c.Terminal.AllowedExternalPaths = mergeSlices(c.Terminal.AllowedExternalPaths, other.Terminal.AllowedExternalPaths)

	// 3. FileSystem
	if other.FileSystem.Enabled {
		c.FileSystem.Enabled = true
	}
	if other.FileSystem.ReadOnly {
		c.FileSystem.ReadOnly = true
	}
	if other.FileSystem.MaxFileSizeKB > 0 {
		c.FileSystem.MaxFileSizeKB = other.FileSystem.MaxFileSizeKB
	}
	c.FileSystem.AllowedPaths = mergeSlices(c.FileSystem.AllowedPaths, other.FileSystem.AllowedPaths)
	c.FileSystem.AllowedExtensions = mergeSlices(c.FileSystem.AllowedExtensions, other.FileSystem.AllowedExtensions)
	c.FileSystem.BlockedFilenames = mergeSlices(c.FileSystem.BlockedFilenames, other.FileSystem.BlockedFilenames)

	// 4. Search
	if other.Search.Enabled {
		c.Search.Enabled = true
	}
	if other.Search.MaxQueryLen > 0 {
		c.Search.MaxQueryLen = other.Search.MaxQueryLen
	}
	c.Search.BlockedSites = mergeSlices(c.Search.BlockedSites, other.Search.BlockedSites)

	// 5. Communication
	if other.Communication.Enabled {
		c.Communication.Enabled = true
	}
	if other.Communication.MaxMessages > 0 {
		c.Communication.MaxMessages = other.Communication.MaxMessages
	}
	if !other.Communication.RequireReview {
		c.Communication.RequireReview = false
	}

	// 6. Network
	if other.Network.Enabled {
		c.Network.Enabled = true
	}
	if other.Network.AllowLanAccess {
		c.Network.AllowLanAccess = true
	}
	if other.Network.AllowInternetAccess {
		c.Network.AllowInternetAccess = true
	}
	if other.Network.MaxFetchSizeKB > 0 {
		c.Network.MaxFetchSizeKB = other.Network.MaxFetchSizeKB
	}
	if other.Network.TimeoutSeconds > 0 {
		c.Network.TimeoutSeconds = other.Network.TimeoutSeconds
	}
	c.Network.BlockedDomains = mergeSlices(c.Network.BlockedDomains, other.Network.BlockedDomains)
	c.Network.BlockedIPs = mergeSlices(c.Network.BlockedIPs, other.Network.BlockedIPs)
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
	ID      string `json:"id"`
	Name    string `json:"name"`
	Key     string `json:"key"`
	BaseURL string `json:"base_url,omitempty"`
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
	Path           string            `json:"path,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	ProviderConfig *ProviderConfig   `json:"provider_config,omitempty"`
	Metadata       *ModelMetadata    `json:"metadata,omitempty"`

	// Agent tuning — per-model overrides for agent loop behaviour.
	// Zero values mean "use the global default."
	MaxSteps            int          `json:"max_steps,omitempty"`
	ContextBudget       int          `json:"context_budget,omitempty"`
	MaxTokens           int          `json:"max_tokens,omitempty"`
	ToolCallFormat      string       `json:"tool_call_format,omitempty"` // "xml" or "native"
	Prefill             *bool        `json:"prefill,omitempty"`
	TimeoutMinutes      int          `json:"timeout_minutes,omitempty"` // per-execution timeout, 0 = use global default (30 min)
	EnableExecutionPlan bool `json:"enable_execution_plan,omitempty"`

	// Resource-aware orchestration. Zero values mean "use provider default."
	ReasoningBudget int `json:"reasoning_budget,omitempty"` // max thinking tokens
	SlotTimeout     int `json:"slot_timeout,omitempty"`     // seconds to keep llama.cpp slot alive
}

type ModelMetadata struct {
	Name          string `json:"name"`
	Architecture  string `json:"architecture"`
	ContextLength int    `json:"context_length"`
	Nctx          int    `json:"n_ctx,omitempty"`
	Parameters    int64  `json:"parameters"`
	Quantization  string `json:"quantization"`
	Author        string `json:"author,omitempty"`
	Description   string `json:"description,omitempty"`
}

type ProviderConfig struct {
	APIKey               string  `json:"-"`
	APIKeyName           string  `json:"api_key_name,omitempty"`
	BaseURL              string  `json:"base_url,omitempty"`
	ProjectID            string  `json:"project_id,omitempty"`
	Region               string  `json:"region,omitempty"`
	InternalCreditWeight float64 `json:"internal_credit_weight,omitempty"` // ICU multiplier per token
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
