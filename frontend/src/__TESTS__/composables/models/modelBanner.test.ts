import { describe, it, expect } from 'vitest'
import { computeModelBanner } from '../../../composables/models/modelBanner'
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
        max_steps: 0,
        context_budget: 0,
        max_tokens: 0,
        temperature: 0,
        reasoning_budget: 0,
        timeout_minutes: 0,
        tool_call_format: '',
        prefill: false,
        tool_timeout_seconds: 0,
        filesystem_tool_timeout_seconds: 0,
        max_plan_duration_minutes: 0,
        max_plan_steps: 0,
        guardrail_timeout_seconds: 0,
        guardrail_timeout_behavior: '',
        guardrail_approval_timeout_seconds: 0,
        loop_strategy: '',
      },
      ...overrides,
    },
  }
}

describe('computeModelBanner', () => {
  it('returns critical when neither primary nor fallback set', () => {
    const b = computeModelBanner(state({}, ['alpha']))
    expect(b?.severity).toBe('critical')
  })

  it('includes html content and an action navigating to the Global settings tab', () => {
    const b = computeModelBanner(state({}, ['alpha']))
    expect(b?.html).toContain('Settings')
    expect(b?.action?.label).toBe('Configure models')
    expect(b?.action?.settingsTab).toBe('local')
  })

  it('fallback notice includes html + a review action', () => {
    const b = computeModelBanner(state({ fallback_model: 'beta' }, ['beta']))
    expect(b?.html).toContain('beta')
    expect(b?.action?.label).toBe('Review models')
  })

  it('returns null when primary is set and in catalogue', () => {
    const b = computeModelBanner(state({ primary_model: 'alpha' }, ['alpha']))
    expect(b).toBeNull()
  })

  it('returns notice when primary unset but fallback ok', () => {
    const b = computeModelBanner(state({ fallback_model: 'beta' }, ['beta']))
    expect(b?.severity).toBe('notice')
    expect(b?.message).toContain('beta')
  })

  it('returns notice when only fallback is set (no primary banner needed)', () => {
    const b = computeModelBanner(state({ fallback_model: 'beta', primary_model: '' }, ['beta']))
    expect(b?.severity).toBe('notice')
  })

  it('treats primary set but not in catalogue as unset (critical)', () => {
    const b = computeModelBanner(state({ primary_model: 'ghost' }, ['alpha']))
    expect(b?.severity).toBe('critical')
  })

  it('returns null when state is null', () => {
    expect(computeModelBanner(null)).toBeNull()
  })
})
