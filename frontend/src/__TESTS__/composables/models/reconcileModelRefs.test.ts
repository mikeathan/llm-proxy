import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useConfig } from '../../../composables/models/useConfig'
import { useModels } from '../../../composables/models/useModels'
import { AdminApiService } from '../../../services/admin/adminService'
import type { AdminState } from '../../../types/admin'

vi.mock('../../../services/admin/adminService', () => ({
  AdminApiService: {
    fetchState: vi.fn(),
  },
}))

function stateWith(
  primary: string,
  fallback: string,
  modelNames: string[],
): AdminState {
  return {
    models: modelNames.map((name) => ({
      name,
      provider: 'openai',
      endpoint: '',
      active: false,
      ready: false,
    })),
    available: [],
    next_port: 9000,
    config: {
      providers: {},
      model_host: '',
      idle_timeout_seconds: 0,
      primary_model: primary,
      fallback_model: fallback,
      guardrails: {
        global: { block_secrets: false, user_blocked_patterns: [] },
        terminal: { enabled: false, allowed_commands: [], timeout_seconds: 0, session_idle_timeout_seconds: 0, max_output_size_chars: 0 },
        search: { enabled: false, max_query_len: 0, blocked_sites: [] },
        communication: { enabled: false, require_review: false, max_messages_per_task: 0 },
        filesystem: { enabled: false, allowed_paths: [], read_only: false, max_file_size_kb: 0 },
        network: { enabled: false, allow_lan_access: false, allow_internet_access: false, max_fetch_size_kb: 0, timeout_seconds: 0 },
      },
      communication: { connectors: {} },
      agent_defaults: {
        max_steps: 0, context_budget: 0, max_tokens: 0, temperature: 0, reasoning_budget: 0,
        timeout_minutes: 0, tool_call_format: '', prefill: false, tool_timeout_seconds: 0,
        filesystem_tool_timeout_seconds: 0, max_plan_duration_minutes: 0, max_plan_steps: 0,
        guardrail_timeout_seconds: 0, guardrail_timeout_behavior: '',
        guardrail_approval_timeout_seconds: 0, loop_strategy: '',
      },
    },
  } as AdminState
}

const { config, reconcileModelRefs } = useConfig()

beforeEach(() => {
  config.value = { ...config.value, primary_model: '', fallback_model: '' }
})

describe('reconcileModelRefs', () => {
  it('clears a primary that no longer exists in the catalogue', () => {
    config.value.primary_model = 'deleted-model'
    reconcileModelRefs(['new-model'])
    expect(config.value.primary_model).toBe('')
  })

  it('clears a dangling fallback while keeping a valid primary', () => {
    config.value.primary_model = 'keep-me'
    config.value.fallback_model = 'deleted-model'
    reconcileModelRefs(['keep-me'])
    expect(config.value.primary_model).toBe('keep-me')
    expect(config.value.fallback_model).toBe('')
  })

  it('leaves valid references untouched', () => {
    config.value.primary_model = 'a'
    config.value.fallback_model = 'b'
    reconcileModelRefs(['a', 'b'])
    expect(config.value.primary_model).toBe('a')
    expect(config.value.fallback_model).toBe('b')
  })
})

describe('useModels.refresh reconciles config model refs', () => {
  it('drops a primary pointing at a model that was deleted', async () => {
    config.value.primary_model = 'deleted-model'
    config.value.fallback_model = 'deleted-model'
    vi.mocked(AdminApiService.fetchState).mockResolvedValue(
      stateWith('deleted-model', 'deleted-model', ['new-model']),
    )

    const { refresh } = useModels()
    await refresh()

    expect(config.value.primary_model).toBe('')
    expect(config.value.fallback_model).toBe('')
  })

  it('keeps a still-valid primary after refresh', async () => {
    config.value.primary_model = 'new-model'
    vi.mocked(AdminApiService.fetchState).mockResolvedValue(
      stateWith('new-model', '', ['new-model']),
    )

    const { refresh } = useModels()
    await refresh()

    expect(config.value.primary_model).toBe('new-model')
  })
})