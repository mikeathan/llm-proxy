// Global configuration types
export type ProviderType = 'local' | 'gemini' | 'openai' | 'openrouter' | 'vertex' | 'mulerouter' | 'nvidia'
export type SettingsTab = ProviderType | 'mcp'

export interface APIKeyItem {
  id: string
  name: string
  key: string
}

export interface ProviderItem {
  type: ProviderType
  api_key?: string // Legacy/Default key
  api_keys?: APIKeyItem[] // Multiple named keys
  base_url?: string
  project_id?: string
  region?: string
  llama_server_binary?: string
  model_dir?: string
  default_args?: string[]
  environment?: Record<string, string>
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
  service_client_id?: string
  service_client_secret?: string
  default_model?: string
  default_args?: string[]
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
