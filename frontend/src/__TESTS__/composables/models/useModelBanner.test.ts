import { describe, it, expect, beforeEach } from 'vitest'
import { nextTick } from 'vue'
import { useAppBanner } from '../../../composables/ui/useAppBanner'
import { useModels } from '../../../composables/models/useModels'
import { useModelBanner } from '../../../composables/models/useModelBanner'
import type { AdminState } from '../../../types/admin'

function state(overrides: Partial<AdminState['config']> = {}, models: string[] = []): AdminState {
  return {
    models: models.map((name) => ({ name, provider: 'local', endpoint: '', active: false, ready: false })),
    available: [],
    next_port: 9000,
    config: {
      providers: {},
      model_host: '',
      idle_timeout_seconds: 0,
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
      },
      ...overrides,
    },
  }
}

describe('useModelBanner', () => {
  const { active: banner, show, clear } = useAppBanner()
  const { state: modelState } = useModels()
  const { recompute } = useModelBanner()

  // The state/transient watchers are registered at module load (self-starting
  // singleton). beforeEach just resets the derived state and the bus, so
  // precedence and re-assert behave exactly as in the app.
  beforeEach(() => {
    modelState.value = state({}, ['alpha'])
    clear()
  })

  it('emits a critical warning when neither primary nor fallback is set', async () => {
    modelState.value = state({}, ['alpha'])
    recompute()
    await nextTick()
    expect(banner.value?.severity).toBe('critical')
    expect(banner.value?.persistent).toBe(true)
    expect(banner.value?.action?.label).toBe('Configure models')
  })

  it('clears the warning once a primary model is configured', async () => {
    modelState.value = state({ primary_model: 'alpha' }, ['alpha'])
    recompute()
    await nextTick()
    expect(banner.value).toBeNull()
  })

  it('shows a notice when only a fallback is set', async () => {
    modelState.value = state({ fallback_model: 'alpha' }, ['alpha'])
    recompute()
    await nextTick()
    expect(banner.value?.severity).toBe('notice')
    expect(banner.value?.action?.label).toBe('Review models')
  })

  it('does not clobber a transient error while it is showing', async () => {
    modelState.value = state({}, ['alpha'])
    recompute()
    await nextTick()
    show({ severity: 'error', message: 'operation failed' })
    await nextTick()
    // A state refresh that would otherwise resolve the warning must not
    // overwrite the visible transient error.
    modelState.value = state({ primary_model: 'alpha' }, ['alpha'])
    await nextTick()
    expect(banner.value?.message).toBe('operation failed')
  })

  it('re-asserts the warning after the transient error clears', async () => {
    modelState.value = state({}, ['alpha'])
    recompute()
    await nextTick()
    show({ severity: 'error', message: 'operation failed' })
    await nextTick()
    clear() // transient dismissed
    await nextTick()
    // The still-unresolved warning re-asserts.
    expect(banner.value?.severity).toBe('critical')
  })
})
