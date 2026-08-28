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

// The agent loop archetype selector. Values mirror the backend
// models.LoopStrategy enum; '' = provider default (react). The UI option list is
// backend-driven (config.loop_strategy_options) and may carry future values not
// in this union — those are handled as raw values by loopStrategyOptions().
export type LoopStrategy = 'react' | 'plan_execute' | 'evaluator_optimizer' | ''

// The known, copy-backed loop-strategy values ('' excluded — it maps to react's
// copy at render time).
export type LoopStrategyOptionKey = Exclude<LoopStrategy, ''>

// One dropdown option for the loop-strategy select. The option list is built
// from the backend-surfaced names (config.loop_strategy_options) by
// loopStrategyOptions(); unknown future values carry the raw value as label.
export interface LoopStrategyOption {
  value: string
  label: string
  description: string
}

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
  guardrail_approval_timeout_seconds?: number
  loop_strategy?: LoopStrategy
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
  guardrail_approval_timeout_seconds?: number
  loop_strategy?: LoopStrategy
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
  guardrail_approval_timeout_seconds?: number
  loop_strategy?: LoopStrategy
}

// TuningSettings is the concrete agent-tuning + safety-timeout set produced by
// getDefaultModelSettings() (provider defaults, all fields required). It is the
// tuning slice of ModelForm minus the local-serving/identity fields.
export interface TuningSettings {
  max_steps: number
  context_budget: number
  max_tokens: number
  temperature: number
  reasoning_budget: number
  reasoning_enabled: boolean
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
}

// ModelForm is the editable shape for the model add/edit form (ModelFormFields).
// Mirrors NewModelForm plus local-serving fields; kept as a discrete form type
// rather than reused request types so UI-only fields (key, port, args) stay
// explicit. Extends TuningSettings so the tuning/safety fields are defined once
// and stay in lockstep with getDefaultModelSettings().
export interface ModelForm extends TuningSettings {
  key: string
  id: string
  name: string
  filename: string
  port: number
  args: string
  slot_timeout: number
}

// ModelBanner is the persistent model-config warning surfaced via the app
// banner. severity uses the shared BannerSeverity union (model banners only
// ever emit 'critical' | 'notice').
export interface ModelBanner {
  severity: import('./ui').BannerSeverity
  message: string
  // HTML variant of the message (app-controlled, never user input).
  html?: string
  // Action button that deep-links to a Settings tab.
  action?: { label: string; settingsTab: import('./admin').SettingsTab }
}

// ProviderManifest describes a provider as discovered/registered by the backend
// for provider-model selection UI.
export interface ProviderManifest {
  id: string
  name: string
  default_base_url: string
  archetype: string
  icon?: string
}

// TuningFieldState classifies a tuning field as editable or derived (read-only,
// server-computed). See composables/settings/useTuningFieldPolicy.ts.
export type TuningFieldState = 'editable' | 'derived'

// TuningFieldPolicy is the field-policy result for the model tuning form,
// driven by the server-computed workload_class (single authority).
export interface TuningFieldPolicy {
  workload: WorkloadClass
  maxTokens: TuningFieldState
  contextBudget: TuningFieldState
  isLocal: boolean
  isCloud: boolean
  isUnresolved: boolean
}
