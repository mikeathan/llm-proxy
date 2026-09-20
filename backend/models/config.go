// Package models defines all shared types for the llm-proxy system:
// model configs, provider definitions, guardrail rules, LLM messages,
// workspace state, and the resource-aware orchestration types.

package models

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

type contextKey string

const (
	WorkspaceIDKey       contextKey = "workspace_id"
	GuardrailApprovedKey contextKey = "guardrail_approved"
	TaskNameKey          contextKey = "task_name"
	RunIDKey             contextKey = "run_id"
	RunNetworkScopeKey   contextKey = "run_network_scope"
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

// WithRunNetworkScope stamps the RESOLVED network scope for an agent run onto
// the context (plan §4.4). Consumers: the guardrail engine (schema + hard
// denial), the network tool config provider (runtime enable/scope), and the
// shell pool key (networkOn). Undecided/absent = the workspace guardrail tier
// governs, preserving pre-grant behavior.
func WithRunNetworkScope(ctx context.Context, scope NetworkScope) context.Context {
	return context.WithValue(ctx, RunNetworkScopeKey, scope)
}

// RunNetworkScopeFrom returns the resolved run scope and whether one was
// explicitly stamped. ok=false (or scope == NetworkScopeInherit) means the
// caller should fall back to the workspace guardrail tier.
func RunNetworkScopeFrom(ctx context.Context) (NetworkScope, bool) {
	if ctx == nil {
		return NetworkScopeInherit, false
	}
	s, ok := ctx.Value(RunNetworkScopeKey).(NetworkScope)
	if !ok || !s.Valid() {
		return NetworkScopeInherit, false
	}
	return s, s != NetworkScopeInherit
}

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

	// present records that the decoded document explicitly contained a network
	// block. It is the override-stack presence signal (SPEC-006 §II.2): a layer
	// that configured the block — including an explicit `false` — replaces the
	// inherited allow flags, while an absent block inherits the baseline.
	// Unset on programmatic construction and never persisted (unexported, so
	// both yaml and json codecs ignore it).
	present bool
}

// UnmarshalYAML records explicit presence of the network block so MergeWith can
// tell "this layer set Internet = false" apart from "this layer said nothing"
// (both decode the boolean as false). Programmatic configs are not overrides and
// correctly leave present unset.
func (c *NetworkGuardrailsConfig) UnmarshalYAML(value *yaml.Node) error {
	// plain drops the method set so value.Decode does not recurse into this
	// unmarshaler.
	type plain NetworkGuardrailsConfig
	var decoded plain
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*c = NetworkGuardrailsConfig(decoded)
	c.present = true
	return nil
}

func (c TerminalGuardrailsConfig) IsActive() bool {
	return c.Enabled || len(c.AllowedCommands) > 0
}

// WithRunScope restricts a copy of this network config to an EXPLICITLY resolved
// run scope (plan §4.4, L2 automation grant): none disables the category; lan /
// internet_only / internet set that exact reachability, overriding the workspace
// L1 allow flags within the host ceiling. `internet` grants LAN + internet;
// `internet_only` grants internet while blocking the in-process LAN tools
// (scan/info) and private-address fetches. The scope-to-config mapping lives
// here (single source) so the guardrail engine, the network tools' runtime
// config, and any future consumer cannot drift. ok=false (inherit/unknown
// scope) returns the config unchanged.
func (c NetworkGuardrailsConfig) WithRunScope(scope NetworkScope) (NetworkGuardrailsConfig, bool) {
	switch scope {
	case NetworkScopeNone:
		c.Enabled = false
		return c, true
	case NetworkScopeLan:
		c.Enabled = true
		c.AllowLanAccess = true
		c.AllowInternetAccess = false
		return c, true
	case NetworkScopeInternetOnly:
		c.Enabled = true
		c.AllowLanAccess = false
		c.AllowInternetAccess = true
		return c, true
	case NetworkScopeInternet:
		c.Enabled = true
		c.AllowLanAccess = true
		c.AllowInternetAccess = true
		return c, true
	default:
		return c, false
	}
}

// EffectiveScope resolves this workspace (L1) guardrail network policy against
// the host ceiling (L0) and an optional per-run (L2) automation grant (plan
// §4.4 grant model). Host off ⇒ none regardless of everything else; an explicit
// grant wins over L1; an empty/unknown grant inherits L1 (Enabled + allow
// flags). The two L1 flags are independent: internet-only when internet is on
// and LAN is off. LAN-vs-internet is an app-layer distinction; the OS shell key
// (D8) only needs NetworkOn (scope != none).
func (c NetworkGuardrailsConfig) EffectiveScope(hostAllowed bool, grant NetworkScope) NetworkScope {
	if !hostAllowed {
		return NetworkScopeNone
	}
	switch grant {
	case NetworkScopeNone, NetworkScopeLan, NetworkScopeInternet, NetworkScopeInternetOnly:
		return grant
	}
	// inherit (or unknown grant) → workspace guardrail L1 scope
	switch {
	case c.Enabled && c.AllowInternetAccess && c.AllowLanAccess:
		return NetworkScopeInternet
	case c.Enabled && c.AllowInternetAccess:
		return NetworkScopeInternetOnly
	case c.Enabled && c.AllowLanAccess:
		return NetworkScopeLan
	default:
		return NetworkScopeNone
	}
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
	// Presence-aware override (SPEC-006 §II.2): a layer that explicitly
	// configured the network block replaces the inherited allow flags, so a
	// workspace can restrict with an explicit `false` as well as loosen. An
	// absent block inherits the baseline. Ints/slices still merge additively.
	if other.Network.present {
		c.Network.Enabled = other.Network.Enabled
		c.Network.AllowLanAccess = other.Network.AllowLanAccess
		c.Network.AllowInternetAccess = other.Network.AllowInternetAccess
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

// ConnectorType values for CommunicationConfig.Connectors[].Type
const (
	ConnectorTypeTelegram = "telegram"
)

// CommunicationConfig holds configuration for all outbound communication connectors.
// Each entry in Connectors maps a user-assigned name (e.g. "my-telegram") to a
// ConnectorConfig that specifies the type, settings, and secret reference.
// This design is intentionally generic — adding a new platform (Slack, Discord, etc.)
// requires no struct changes, only a new case in the registry switch.
type CommunicationConfig struct {
	Connectors map[string]ConnectorConfig `json:"connectors"`
}

// ConnectorConfig defines a single communication channel.
// Type is the platform identifier (e.g. "telegram", "slack").
// Settings is a type-specific key-value map (e.g. {"chat_id": "12345"}).
// SecretRef points to the secrets store key holding the auth token/credential.
type ConnectorConfig struct {
	Type       string            `json:"type"`
	Enabled    bool              `json:"enabled"`
	Settings   map[string]string `json:"settings"`
	SecretRef  string            `json:"secret_ref,omitempty"`
	WebhookURL string            `json:"webhook_url,omitempty"`
}

// SearchProvider is a fixed-value enum of the supported internet-search
// backends. Persisted in registry.json, so the wire value is the JSON string
// (never persisted in yaml).
type SearchProvider string

const (
	SearchProviderTavily  SearchProvider = "tavily"
	SearchProviderBrave   SearchProvider = "brave"
	SearchProviderSerpAPI SearchProvider = "serpapi"
)

// Search result bounds. MaxSearchMaxResults is the ceiling the save boundary
// enforces; DefaultSearchMaxResults is what the resolver applies when the
// operator left max_results unset.
const (
	DefaultSearchMaxResults = 5
	MaxSearchMaxResults     = 20
)

// ErrInvalidSearchProvider and ErrSearchMaxResultsOutOfRange are the
// save-boundary validation failures for SearchConfig. IsSearchConfigError
// classifies both so transport handlers map them in one check.
var (
	ErrInvalidSearchProvider      = errors.New("invalid search provider")
	ErrSearchMaxResultsOutOfRange = errors.New("search max_results out of range")
)

// SearchProviderIDs returns the canonical, ordered provider list (Tavily first).
// The backend surfaces it so the frontend never hardcodes the option set.
func SearchProviderIDs() []SearchProvider {
	return []SearchProvider{SearchProviderTavily, SearchProviderBrave, SearchProviderSerpAPI}
}

// Valid reports whether p is a registered provider. The empty string is valid —
// it means "unset", which the resolver maps to the default provider.
func (p SearchProvider) Valid() bool {
	switch p {
	case "", SearchProviderTavily, SearchProviderBrave, SearchProviderSerpAPI:
		return true
	default:
		return false
	}
}

// SearchConfig selects the internet-search backend and its result cap. It lives
// in registry.json (Tier 3) alongside the rest of the dynamic search state.
type SearchConfig struct {
	Provider   SearchProvider `json:"provider,omitempty"`
	MaxResults int            `json:"max_results,omitempty"`
}

// Validate enforces the save-boundary contract: an unset provider and a zero
// max_results are valid (defaults apply); an unknown provider or an
// out-of-range max_results is rejected.
func (c SearchConfig) Validate() error {
	if !c.Provider.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidSearchProvider, c.Provider)
	}
	if c.MaxResults != 0 && (c.MaxResults < 1 || c.MaxResults > MaxSearchMaxResults) {
		return fmt.Errorf("%w: %d (want 1..%d)", ErrSearchMaxResultsOutOfRange, c.MaxResults, MaxSearchMaxResults)
	}
	return nil
}

// IsSearchConfigError reports whether err is a search-config validation failure,
// so handlers can map the whole class to a 400 in one predicate.
func IsSearchConfigError(err error) bool {
	return errors.Is(err, ErrInvalidSearchProvider) || errors.Is(err, ErrSearchMaxResultsOutOfRange)
}

// DefaultSearchConfig is the resolver's fallback when the operator has not
// chosen a provider (or left max_results unset).
func DefaultSearchConfig() SearchConfig {
	return SearchConfig{
		Provider:   SearchProviderTavily,
		MaxResults: DefaultSearchMaxResults,
	}
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

	// WorkloadClass is a computed field resolved at the runtime boundary after
	// provider configuration and secret hydration.  Never persisted — a stored
	// value goes stale the moment a base URL changes.  Budget, ICU, and the
	// reasoning wire all key on it (single authority).
	WorkloadClass WorkloadClass `json:"-" yaml:"-"`

	// Published capabilities resolved from the live provider catalog (when a
	// provider publishes them).  Computed-only; never persisted.
	PublishedContextLength int `json:"-" yaml:"-"`
	PublishedOutputCap     int `json:"-" yaml:"-"`

	// Agent tuning — per-model overrides for agent loop behaviour.
	// Zero values mean "use the global default."
	MaxSteps         int          `json:"max_steps,omitempty"`
	ContextBudget    int          `json:"context_budget,omitempty"`
	MaxTokens        int          `json:"max_tokens,omitempty"`
	Temperature      float64      `json:"temperature,omitempty"`      // overrides the 0.1 automation default
	ToolCallFormat   string       `json:"tool_call_format,omitempty"` // "xml" or "native"
	Prefill          *bool        `json:"prefill,omitempty"`
	ReasoningEnabled *bool        `json:"reasoning_enabled,omitempty"` // provider-native reasoning toggle
	TimeoutMinutes   int          `json:"timeout_minutes,omitempty"`   // per-execution timeout, 0 = use global default (30 min)
	LoopStrategy     LoopStrategy `json:"loop_strategy,omitempty"`     // agent loop archetype; "" = provider default / react

	// Resource-aware orchestration. Zero values mean "use provider default."
	ReasoningBudget int `json:"reasoning_budget,omitempty"` // max thinking tokens
	SlotTimeout     int `json:"slot_timeout,omitempty"`     // seconds to keep llama.cpp slot alive

	// Safety timeouts — per-model overrides. Zero means "use global default."
	ToolTimeoutSeconds           int    `json:"tool_timeout_seconds,omitempty"`               // default 120 (2 min), 0 = disabled
	FilesystemToolTimeoutSeconds int    `json:"filesystem_tool_timeout_seconds,omitempty"`    // default 30
	MaxPlanDurationMinutes       int    `json:"max_plan_duration_minutes,omitempty"`          // default 15
	MaxPlanSteps                 int    `json:"max_plan_steps,omitempty"`                     // default 50
	GuardrailTimeoutSeconds      int    `json:"guardrail_timeout_seconds,omitempty"`          // default 5
	GuardrailTimeoutBehavior     string `json:"guardrail_timeout_behavior,omitempty"`         // "fail-open" | "fail-closed"
	GuardrailApprovalTimeoutSecs int    `json:"guardrail_approval_timeout_seconds,omitempty"` // default 300
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
	// MaxOutputTokens is the published per-model output cap from the provider's
	// live catalog, persisted with the model so the cloud clamp survives a
	// restart (Phase 2 — "carry MaxOutputTokens into ModelMetadata").  0 means
	// unknown; the tier row remains the fallback.
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
}

type ProviderConfig struct {
	APIKey               string  `json:"-"`
	APIKeyName           string  `json:"api_key_name,omitempty"`
	BaseURL              string  `json:"base_url,omitempty"`
	ProjectID            string  `json:"project_id,omitempty"`
	Region               string  `json:"region,omitempty"`
	InternalCreditWeight float64 `json:"internal_credit_weight,omitempty"` // ICU multiplier per token
}

// MetricsConfig carries the GPU metrics display settings. The yaml tags use the
// documented snake_case keys (settings.yml); UnmarshalYAML (metrics_config_yaml.go)
// also accepts the pre-tag field-name-derived keys so existing files keep loading.
type MetricsConfig struct {
	GPU                  GPUConfig `yaml:"gpu" json:"gpu"`
	GPUSampleIntervalSec int       `yaml:"gpu_sample_interval_seconds" json:"gpu_sample_interval_seconds,omitempty"`
	GPUSmoothingAlpha    float64   `yaml:"gpu_smoothing_alpha" json:"gpu_smoothing_alpha,omitempty"`
}

type GPUConfig struct {
	Provider  string `yaml:"provider" json:"provider,omitempty"`
	Binary    string `yaml:"binary" json:"binary,omitempty"`
	Index     int    `yaml:"index" json:"index,omitempty"`
	SysfsPath string `yaml:"sysfs_path" json:"sysfs_path,omitempty"`
}
