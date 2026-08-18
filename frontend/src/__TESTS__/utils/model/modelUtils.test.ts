import { describe, it, expect } from 'vitest'
import {
  loopStrategyOptions,
  loopStrategyDescription,
  getDefaultModelSettings,
  LOOP_STRATEGY_COPY,
} from '../../../utils/model/modelUtils'
import type { AgentDefaults } from '../../../types/admin'
import type { LoopStrategy } from '../../../types/model'

function agentDefaults(overrides: Partial<AgentDefaults> = {}): AgentDefaults {
  return {
    max_steps: 25,
    context_budget: 8000,
    max_tokens: 3072,
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
    guardrail_approval_timeout_seconds: 300,
    loop_strategy: '',
    ...overrides,
  }
}

describe('loopStrategyOptions', () => {
  it('falls back to the known values when the backend list is empty', () => {
    const opts = loopStrategyOptions([])
    expect(opts).toHaveLength(3)
    const values = opts.map((o) => o.value)
    expect(values).toEqual(['react', 'plan_execute', 'evaluator_optimizer'])
    expect(opts[0]).toMatchObject({ value: 'react', label: LOOP_STRATEGY_COPY.react.label })
  })

  it('falls back to the known values when no list is provided', () => {
    expect(loopStrategyOptions()).toHaveLength(3)
  })

  it('builds options from the backend-surfaced names with copy', () => {
    const opts = loopStrategyOptions(['react', 'plan_execute'])
    expect(opts).toHaveLength(2)
    expect(opts[0]).toMatchObject({
      value: 'react',
      label: LOOP_STRATEGY_COPY.react.label,
      description: LOOP_STRATEGY_COPY.react.description,
    })
    expect(opts[1]).toMatchObject({ value: 'plan_execute', label: LOOP_STRATEGY_COPY.plan_execute.label })
  })

  it('renders unknown values raw with no description', () => {
    const opts = loopStrategyOptions(['future_archetype'])
    expect(opts).toHaveLength(1)
    expect(opts[0]).toEqual({ value: 'future_archetype', label: 'future_archetype', description: '' })
  })
})

describe('loopStrategyDescription', () => {
  it('maps empty to react copy (provider default)', () => {
    expect(loopStrategyDescription('')).toBe(LOOP_STRATEGY_COPY.react.description)
  })

  it('returns the selected strategy description', () => {
    expect(loopStrategyDescription('plan_execute')).toBe(LOOP_STRATEGY_COPY.plan_execute.description)
    expect(loopStrategyDescription('evaluator_optimizer')).toBe(
      LOOP_STRATEGY_COPY.evaluator_optimizer.description,
    )
  })

  it('returns empty for unknown values (defensive wire case)', () => {
    // The model field is typed LoopStrategy, but the wire may carry a future
    // backend value not yet in the union — the function must degrade gracefully.
    expect(loopStrategyDescription('nonsense' as LoopStrategy)).toBe('')
  })
})

describe('getDefaultModelSettings', () => {
  it('carries the agent-defaults loop_strategy (empty = provider default react)', () => {
    const settings = getDefaultModelSettings('local', agentDefaults())
    expect(settings.loop_strategy).toBe('')

    const withOverride = getDefaultModelSettings('openai', agentDefaults({ loop_strategy: 'plan_execute' }))
    expect(withOverride.loop_strategy).toBe('plan_execute')
  })

  it('carries the agent-defaults guardrail approval timeout', () => {
    const settings = getDefaultModelSettings('local', agentDefaults())
    expect(settings.guardrail_approval_timeout_seconds).toBe(300)

    const withOverride = getDefaultModelSettings('openai', agentDefaults({ guardrail_approval_timeout_seconds: 600 }))
    expect(withOverride.guardrail_approval_timeout_seconds).toBe(600)
  })
})
