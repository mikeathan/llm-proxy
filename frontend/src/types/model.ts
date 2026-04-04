// LLM Model related types
export interface ActiveModel {
  name: string
  endpoint: string
  port: number
  ready: boolean
  started_at: string
  last_used_at: string
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
}

export interface AvailableModel {
  name: string
  filename: string
  resolved_path: string
}

export interface NewModelForm {
  name: string
  provider: import('./admin').ProviderType
  model_id?: string
  filename?: string
  port?: number
  args?: string
}
