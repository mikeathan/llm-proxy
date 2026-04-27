// Global configuration types
export type ProviderType = 'local' | 'gemini' | 'openai' | 'openrouter' | 'vertex' | 'mulerouter' | 'nvidia'
export type SettingsTab = ProviderType | 'mcp' | 'guardrails' | 'security' | 'catalogue'

export interface APIKeyItem {
  id: string
  name: string
  key: string
}

export interface TerminalGuardrailsConfig {
  enabled: boolean
  allowed_commands: string[]
  allowed_env_vars?: string[]
  blocked_patterns?: string[]
  path_extensions?: string[]
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
  primary_model?: string
  fallback_model?: string
  default_args?: string[]
  guardrails: AgentGuardrailsConfig
  communication: {
    telegram: {
      enabled: boolean
      chat_id: string
    }
  }
}

export interface AgentDefinition {
  id: string
  name: string
  provider_id: string
  model_id: string
  system_prompt: string
  tools: string[]
}

export interface AdminState {
  models: import('./model').Model[]
  available: import('./model').AvailableModel[]
  next_port: number
  active?: import('./model').ActiveModel
  config: GlobalConfig
}
