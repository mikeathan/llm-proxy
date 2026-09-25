// Global configuration types
import type { LoopStrategy, Model, AvailableModel, ActiveModel } from './model'

export type ProviderType = 'local' | 'gemini' | 'openai' | 'openrouter' | 'nvidia'
export type SettingsTab = ProviderType | 'local-models' | 'mcp' | 'guardrails' | 'security' | 'processes' | 'communication' | 'search'

export interface APIKeyItem {
  id: string
  name: string
  key: string
  base_url?: string
}

export interface TerminalGuardrailsConfig {
  enabled: boolean
  allowed_commands: string[]
  allowed_env_vars?: string[]
  blocked_patterns?: string[]
  path_extensions?: string[]
  allowed_external_paths?: string[]
  timeout_seconds: number
  session_idle_timeout_seconds: number
  max_output_size_chars: number
}

export interface FileSystemGuardrailsConfig {
  enabled: boolean
  allowed_paths: string[]
  read_only: boolean
  max_file_size_kb: number
  allowed_extensions?: string[]
  blocked_filenames?: string[]
}

export interface SearchGuardrailsConfig {
  enabled: boolean
  max_query_len: number
  blocked_sites: string[]
}

export interface CommunicationGuardrailsConfig {
  enabled: boolean
  require_review: boolean
  max_messages_per_task: number
}

export interface NetworkGuardrailsConfig {
  enabled: boolean
  allow_lan_access: boolean
  allow_internet_access: boolean
  blocked_domains?: string[]
  blocked_ips?: string[]
  max_fetch_size_kb: number
  timeout_seconds: number
}

export interface GlobalGuardrailsConfig {
  block_secrets: boolean
  user_blocked_patterns: string[]
}

export interface AgentGuardrailsConfig {
  global: GlobalGuardrailsConfig
  terminal: TerminalGuardrailsConfig
  search: SearchGuardrailsConfig
  communication: CommunicationGuardrailsConfig
  filesystem: FileSystemGuardrailsConfig
  network: NetworkGuardrailsConfig
}

export interface ProviderItem {
  type: ProviderType
  base_url?: string
  project_id?: string
  region?: string
  llama_server_binary?: string
  model_dir?: string
  default_args?: string[]
  environment?: Record<string, string>
  api_keys?: APIKeyItem[]
}

// Global configuration types
export interface ConnectorConfig {
  type: string
  enabled: boolean
  settings: Record<string, string>
  secret_ref?: string
  webhook_url?: string
}

export interface CommunicationConfig {
  connectors: Record<string, ConnectorConfig>
}

// Internet-search backend selection (backend models.SearchProvider enum). The
// live option list is surfaced by the backend as config.search_providers — this
// union covers the known values and helpers degrade gracefully for unknowns.
export type SearchProvider = 'tavily' | 'brave' | 'serpapi'

export interface SearchConfig {
  provider?: SearchProvider
  max_results?: number
}

export interface WebhookInfo {
  url: string
  pending_updates: number
  last_error?: string
}

export type VerifyState = 'idle' | 'checking' | 'registered' | 'unregistered' | 'error'

export interface WebhookUIState {
  host: string
  info: WebhookInfo | null
  creating: boolean
  verifying: boolean
  deleting: boolean
  verifyState: VerifyState
  verifyMsg: string
  statusMsg: string
}

// ReasoningCapability describes whether (and how) a provider's reasoning toggle
// can be rendered. Surfaced by the backend per provider via provider_defaults;
// the UI never hardcodes provider names.
export interface ReasoningCapability {
  supported: boolean
  toggleable: boolean
  default_enabled: boolean
  mode: string
}

export interface AgentDefaults {
  max_steps: number
  context_budget: number
  max_tokens: number
  temperature: number
  reasoning_budget: number
  timeout_minutes: number
  tool_call_format: string
  prefill: boolean
  tool_timeout_seconds: number
  filesystem_tool_timeout_seconds: number
  max_plan_duration_minutes: number
  max_plan_steps: number
  guardrail_timeout_seconds: number
  guardrail_timeout_behavior: string
  guardrail_approval_timeout_seconds: number
  loop_strategy: LoopStrategy
  reasoning?: ReasoningCapability
  // supports_base_url is surfaced by the backend per provider (provider_defaults);
  // it replaces the previously hardcoded OpenAI-compatible provider list.
  supports_base_url?: boolean
}

export interface GlobalConfig {
  providers: Record<string, ProviderItem>
  agents?: AgentDefinition[]
  workspaces_dir?: string
  model_host: string
  idle_timeout_seconds: number
  gpu_provider?: string
  gpu_binary?: string
  gpu_index?: number
  gpu_sysfs_path?: string
  gpu_sample_interval_seconds?: number
  gpu_smoothing_alpha?: number
  primary_model?: string
  fallback_model?: string
  default_args?: string[]
  guardrails: AgentGuardrailsConfig
  communication: CommunicationConfig
  search?: SearchConfig
  // Backend-driven search-provider option list (mirrors loop_strategy_options).
  // The Settings dropdown is driven by this; constants/search.ts is a fallback.
  search_providers?: string[]
  agent_defaults: AgentDefaults
  provider_defaults?: Record<string, AgentDefaults>
  loop_strategy_options?: string[]
  run_logging?: { enabled: boolean }
  // Run scheduler admission limits (global-run-lane plan).
  scheduler?: SchedulerConfig
}

export interface SchedulerConfig {
  local_concurrency: number
  cloud_concurrency: number
  preempt_automations: boolean
  // Inbound admission for external /v1 callers asking for a local model.
  // seconds a caller may wait: 0 refuses immediately, -1 waits indefinitely.
  inbound_wait_seconds?: number
  // How many callers may wait at once (always enforced).
  inbound_max_queued?: number
  // Whether a contended caller may cancel the run holding the model. Off by
  // default: otherwise only the operator's "Serve now" evicts.
  inbound_preempt?: boolean
}

export interface AgentDefinition {
  id: string
  name: string
  provider_id: string
  model_id: string
  system_prompt: string
  tools: string[]
}

export interface ProcessInfo {
  pid: number
  binary: string
  model?: string
  port?: number
  started: string
  uptime: string
  active: boolean
}

export interface ProcessListResponse {
  processes: ProcessInfo[]
}

export interface ProcessKillResponse {
  status: string
  pid: number
}

export interface AdminState {
	models: Model[]
	available: AvailableModel[]
	next_port: number
	active?: ActiveModel
	config: GlobalConfig
}

// Host sandboxing (backend models.HostSandboxingConfig, host-settings GET/PUT).
// filesystem/network are optional booleans: absent (undefined) = undecided —
// filesystem undecided means ON (jail active), network undecided means ALLOWED
// (legacy) and drives the migration banner. egress_proxy is 0 (off) or a port.
export interface SandboxingConfig {
  enabled: boolean
  functional: boolean
  max_memory_mb: number
  max_storage_gb: number
  filesystem?: boolean
  network?: boolean
  egress_proxy: number
  egress_allow_domains?: string[]
  egress_deny_domains?: string[]
}

export interface HostSettings {
  sandboxing: SandboxingConfig
  // Runtime projection of what this host ACTUALLY enforces (backend
  // models.SandboxEffective, SPEC-006 §II.7.4). Optional — older backends (or
  // the moment before the provider is wired) omit it. Never persisted; a PUT
  // must not send it back.
  effective?: SandboxEffective
}

// Backend models.SandboxEffective — mechanism per surface plus the reason when
// the requested enforcement is not available on this host ("downgrade never
// silent"). Mechanism values: 'none' | 'landlock' | 'seatbelt' | 'bwrap' (or a
// future OS mechanism).
export interface SandboxEffective {
  filesystem: SandboxSurface
  network: SandboxSurface
  provider: string
}

export interface SandboxSurface {
  mechanism: string
  reason?: string
}

// Terminal session view (backend models.TerminalSessionView, GET
// /admin/api/terminals/sessions). A workspace can hold up to two pooled
// sessions (network on/off, plan D8) — network_on disambiguates the pool key.
export interface TerminalSessionView {
  workspace_id: string
  last_used: string
  host_path: string
  network_on: boolean
}

// Per-automation network grant (backend models.NetworkScope): '' = inherit the
// workspace scope; 'none' | 'lan' | 'internet_only' | 'internet' override it for
// that run. 'lan' = local network only; 'internet_only' = internet with the
// local network blocked; 'internet' = local network + internet.
export type NetworkGrant = '' | 'none' | 'lan' | 'internet_only' | 'internet'

// Host network master-switch state as read for run-grant warnings: 'on'/'off'
// once the host settings respond, 'unknown' before/if they do not.
export type HostNetworkState = 'on' | 'off' | 'unknown'
