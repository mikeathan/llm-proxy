// Global configuration types
export interface GlobalConfig {
  model_dir: string
  llama_binary: string
  model_host: string
  idle_timeout_seconds: number
  gpu_provider?: string
  gpu_binary?: string
  gpu_index?: number
  service_client_id?: string
  service_client_secret?: string
  environment: Record<string, string>
  default_args: string[]
}

export interface AdminState {
  models: import('./model').Model[]
  available: import('./model').AvailableModel[]
  next_port: number
  active?: import('./model').ActiveModel
  config: GlobalConfig
}
