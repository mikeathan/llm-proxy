import { ref } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import type { GlobalConfig } from '../../types/admin'

export const DEFAULT_CONFIG: GlobalConfig = {
  providers: {
    local: {
      type: 'local',
      model_dir: '',
      llama_server_binary: '',
      default_args: [],
      environment: {},
    },
  },
  model_host: '127.0.0.1',
  idle_timeout_seconds: 1800,
  guardrails: {
    global: { block_secrets: true, user_blocked_patterns: [] },
    terminal: {
      enabled: false,
      allowed_commands: [],
      timeout_seconds: 30,
      session_idle_timeout_seconds: 1800,
      max_output_size_chars: 10000,
    },
    search: { enabled: false, max_query_len: 200, blocked_sites: [] },
    communication: {
      enabled: false,
      require_review: true,
      max_messages_per_task: 10,
    },
    network: {
      enabled: false,
      allow_lan_access: false,
      allow_internet_access: false,
      max_fetch_size_kb: 5000,
      timeout_seconds: 30,
    },
    filesystem: {
      enabled: false,
      allowed_paths: [],
      read_only: true,
      max_file_size_kb: 1024,
    },
  },
  communication: {
    connectors: {},
  },
  agent_defaults: {
    max_steps: 25,
    context_budget: 8000,
    max_tokens: 2048,
    temperature: 0.1,
    reasoning_budget: 0,
    timeout_minutes: 30,
    tool_call_format: '',
    prefill: false,
    tool_timeout_seconds: 120,
    filesystem_tool_timeout_seconds: 30,
    max_plan_duration_minutes: 15,
    max_plan_steps: 50,
    guardrail_timeout_seconds: 5,
    guardrail_timeout_behavior: 'fail-open',
  },
  run_logging: { enabled: false },
}

// Global state to share across components
const config = ref<GlobalConfig>({ ...DEFAULT_CONFIG })
const isSaving = ref(false)
const isLoading = ref(false)
const error = ref<string | null>(null)

/**
 * Fetches the latest config from the backend.
 */
const fetchConfig = async (): Promise<void> => {
  isLoading.value = true
  error.value = null
  try {
    const state = await AdminApiService.fetchState()
    if (state.config) {
      config.value = structuredClone(state.config)
    }
  } catch (err: any) {
    error.value = err.message || 'Failed to fetch configuration'
    console.error('[useConfig] fetch failed:', err)
  } finally {
    isLoading.value = false
  }
}

/**
 * Updates the global config.
 */
const updateConfig = async (payload?: GlobalConfig): Promise<void> => {
  isSaving.value = true
  error.value = null
  const data = payload || config.value
  try {
    await AdminApiService.updateConfig(data)
  } catch (err: any) {
    error.value = err.message || 'Failed to save configuration'
    throw err
  } finally {
    isSaving.value = false
  }
}

/**
 * Helper to ensure a specific provider exists in the config.
 */
const ensureProvider = (type: string) => {
  if (!config.value.providers) config.value.providers = {}
  if (!config.value.providers[type]) {
    config.value.providers[type] = { type: type as any, api_keys: [] }
  }
  return config.value.providers[type]
}

export function useConfig() {
  return {
    config,
    isLoading,
    isSaving,
    error,
    fetchConfig,
    updateConfig,
    ensureProvider
  }
}
