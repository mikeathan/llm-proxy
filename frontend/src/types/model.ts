// LLM Model related types
export interface ActiveModel {
  name: string
  provider: import('./admin').ProviderType
  endpoint: string
  port: number
  ready: boolean
  started_at: string
  last_used_at: string
}

export interface ProviderConfig {
  api_key?: string
  api_key_name?: string
  base_url?: string
  project_id?: string
  region?: string
  internal_credit_weight?: number
}

export interface Model {
  name: string
  provider: import('./admin').ProviderType
  model_id?: string
  filename?: string
  resolved_path?: string
  args?: string[]
  port?: number
  endpoint: string
  active: boolean
  ready: boolean
  provider_config?: ProviderConfig
  metadata?: ModelMetadata
  prefill?: boolean
  max_steps?: number
  context_budget?: number
  max_tokens?: number
  reasoning_budget?: number
  slot_timeout?: number
  tool_call_format?: string
}

export interface ModelMetadata {
  name: string
  architecture: string
  context_length: number
  parameters: number
  quantization: string
  author?: string
  description?: string
}

export interface AvailableModel {
  name: string
  filename: string
  resolved_path: string
  size_bytes: number
  metadata: ModelMetadata
}

export interface ModelPricing {
  prompt: string
  completion: string
}

export interface ModelLimits {
  context?: number
}

export interface ProviderModelInfo {
  id: string
  pricing?: ModelPricing
  limits?: ModelLimits
}

export interface NewModelForm {
  name: string
  provider: import('./admin').ProviderType
  model_id?: string
  filename?: string
  port?: number
  args?: string
  provider_config?: ProviderConfig
  metadata?: ModelMetadata
  prefill?: boolean
  max_steps?: number
  context_budget?: number
  max_tokens?: number
  reasoning_budget?: number
  slot_timeout?: number
  tool_call_format?: string
}
