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

// Workload class as computed by the backend (single authority): local workloads
// derive max_tokens/context_budget from serving n_ctx; cloud workloads are
// editable and prefilled from published capabilities.
export type WorkloadClass = 'local' | 'cloud' | ''

// The agent-tuning + safety-timeout fields edited by ModelTuningFields.vue.
// All fields optional so both the add-form ModelForm and the edit
// Partial<Model> satisfy it; omitted fields mean "use provider default".
export interface TuningFields {
  max_steps?: number
  context_budget?: number
  max_tokens?: number
  reasoning_budget?: number
  reasoning_enabled?: boolean
  temperature?: number
  timeout_minutes?: number
  tool_call_format?: string
  prefill?: boolean
  tool_timeout_seconds?: number
  filesystem_tool_timeout_seconds?: number
  max_plan_duration_minutes?: number
  max_plan_steps?: number
  guardrail_timeout_seconds?: number
  guardrail_timeout_behavior?: string
}

export interface Model {
  name: string
  provider: import('./admin').ProviderType
  workload_class?: WorkloadClass
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
  reasoning_enabled?: boolean
  max_steps?: number
  context_budget?: number
  max_tokens?: number
  temperature?: number
  reasoning_budget?: number
  slot_timeout?: number
  timeout_minutes?: number
  tool_call_format?: string
  tool_timeout_seconds?: number
  filesystem_tool_timeout_seconds?: number
  max_plan_duration_minutes?: number
  max_plan_steps?: number
  guardrail_timeout_seconds?: number
  guardrail_timeout_behavior?: string
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

export interface ModelMeta {
  n_ctx_train?: number
  n_ctx?: number
  n_params?: number
}

export interface ProviderModelInfo {
  id: string
  pricing?: ModelPricing
  limits?: ModelLimits
  meta?: ModelMeta
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
  reasoning_enabled?: boolean
  max_steps?: number
  context_budget?: number
  max_tokens?: number
  temperature?: number
  reasoning_budget?: number
  slot_timeout?: number
  timeout_minutes?: number
  tool_call_format?: string
  tool_timeout_seconds?: number
  filesystem_tool_timeout_seconds?: number
  max_plan_duration_minutes?: number
  max_plan_steps?: number
  guardrail_timeout_seconds?: number
  guardrail_timeout_behavior?: string
}
