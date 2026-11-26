import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import ModelTuningFields from '../../../components/settings/ModelTuningFields.vue'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import { loopStrategyDescription } from '../../../utils/model/modelUtils'
import type { TuningFields, WorkloadClass, LoopStrategyOption } from '../../../types/model'
import type { ReasoningCapability } from '../../../types/admin'

const STRATEGIES: LoopStrategyOption[] = [
  { value: 'react', label: 'ReAct', description: 'Think, act, observe.' },
  { value: 'plan_execute', label: 'Plan first', description: 'Plan the whole task, then execute.' },
]

// The fields edit the form object the parent passes in (the add / edit model
// draft), so the draft is the observable contract.
function mountTuning(workloadClass: WorkloadClass, reasoning?: ReasoningCapability, model: TuningFields = {}) {
  const draft = reactive<TuningFields>({ ...model })
  const w = mount(ModelTuningFields, {
    props: { model: draft, provider: workloadClass === 'local' ? 'local' : 'openai', workloadClass, reasoning, loopStrategyOptions: STRATEGIES },
    attachTo: document.body,
    global: { stubs: { Icon: true } },
  })
  return { w, draft }
}

describe('ModelTuningFields', () => {
  // A local model's output cap, reasoning budget and context budget are set by the server (it clears submitted ones), so they are one
  // sentence, never inputs: nothing there can block Save (pitfall 78).
  it('shows the limits the server sets for a local model as a sentence, with no inputs', () => {
    const { w } = mountTuning('local', undefined, { max_tokens: 87381, reasoning_budget: 29127, context_budget: 699052 })
    const text = w.get('[data-test="derived-limits"]').text()
    expect(text).toContain('Output up to 87.4K tokens')
    expect(text).toContain('thinking 29.1K')
    expect(text).toContain('history 699.1K characters')
    expect(text).toContain('set by the server')
    for (const label of [/max tokens/i, /context budget/i, /reasoning budget/i]) expect(() => fieldByLabel(w, label)).toThrow()
  })

  it('says a new local model gets its limits when it is saved', () => {
    expect(mountTuning('local').w.get('[data-test="derived-limits"]').text()).toContain('when it is saved')
  })

  it('lets a cloud model set its reasoning budget', () => {
    expect(fieldByLabel(mountTuning('cloud').w, /reasoning budget/i).attributes('readonly')).toBeUndefined()
  })

  it('lets cloud models set the context budget and output cap', async () => {
    const { w, draft } = mountTuning('cloud')
    expect(fieldByLabel(w, /max tokens/i).attributes('readonly')).toBeUndefined()
    await fieldByLabel(w, /max tokens/i).setValue('4096')
    expect(draft.max_tokens).toBe(4096)
  })

  it('offers the thinking switch only for cloud models whose reasoning is toggleable', () => {
    const labels = (w: ReturnType<typeof mountTuning>['w']) => w.findAll('label').map((l) => l.text()).join('|')
    expect(labels(mountTuning('cloud', { toggleable: true, mode: 'effort' } as ReasoningCapability).w)).toMatch(/enable thinking/i)
    expect(labels(mountTuning('cloud', { toggleable: false } as ReasoningCapability).w)).not.toMatch(/enable thinking/i)
    expect(labels(mountTuning('local', { toggleable: true } as ReasoningCapability).w)).not.toMatch(/enable thinking/i)
  })

  it('describes the selected loop strategy', async () => {
    const { w } = mountTuning('cloud', undefined, { loop_strategy: 'plan_execute' })
    const select = fieldByLabel(w, /loop strategy/i)
    expect(select.findAll('option').map((o) => o.text())).toEqual(['Provider default (ReAct)', 'ReAct', 'Plan first'])
    expect(w.text()).toContain(loopStrategyDescription('plan_execute'))
  })

  // A cloud model's limits are prefilled by the server, so the browser's own step and range checks must accept them: one that does not
  // blocks Save with "Please select a valid value" (29127 is 87381 / 3, not a multiple of 128).
  it('accepts the values the server fills in, so Save is not blocked', () => {
    for (const workload of ['cloud', ''] as WorkloadClass[]) {
      // Values the server derived for real models: n_ctx 262144 (Qwen), 1048576 (Inkling) and a cloud cap.
      for (const values of [
        { max_tokens: 87381, reasoning_budget: 29127, context_budget: 699052 },
        { max_tokens: 349525, reasoning_budget: 116508, context_budget: 2796204 },
        { max_tokens: 5461, reasoning_budget: 1820, context_budget: 43692 },
      ]) {
        const { w } = mountTuning(workload, undefined, values)
        for (const label of [/max tokens/i, /reasoning budget/i, /context budget/i]) {
          const input = fieldByLabel(w, label).element as HTMLInputElement
          expect(input.validity.valid, `${workload} ${label} ${input.value}`).toBe(true)
        }
        w.unmount()
      }
    }
  })

  // Grouping folded the rarely changed fields away; every one must still be there to find and change.
  it('keeps every tuning field reachable, the folded ones included', () => {
    const ALWAYS = [/^max steps/i, /^temperature/i, /loop strategy/i, /prefill/i, /tool call format/i, /^timeout \(min\)/i, /per-tool timeout/i, /filesystem timeout/i, /max plan duration/i, /max plan steps/i, /guardrail timeout \(sec\)/i, /guardrail timeout behavio/i, /guardrail approval timeout/i]
    const CLOUD_ONLY = [/^max tokens/i, /context budget/i, /reasoning budget/i]
    const local = mountTuning('local').w
    for (const label of ALWAYS) expect(() => fieldByLabel(local, label), String(label)).not.toThrow()
    const cloud = mountTuning('cloud').w
    for (const label of [...ALWAYS, ...CLOUD_ONLY]) expect(() => fieldByLabel(cloud, label), String(label)).not.toThrow()
  })

  it('says how many of the folded settings are set', () => {
    expect(mountTuning('cloud').w.text()).toContain('all default')
    expect(mountTuning('cloud', undefined, { timeout_minutes: 30, guardrail_timeout_behavior: 'fail-closed' }).w.text()).toContain('2 set')
  })

  // An unset select showed as a blank box; it shows its default option instead.
  it('shows the default option of a select whose field is unset', () => {
    const { w } = mountTuning('local')
    expect((fieldByLabel(w, /loop strategy/i).element as HTMLSelectElement).selectedOptions[0]?.text).toBe('Provider default (ReAct)')
    expect((fieldByLabel(w, /tool call format/i).element as HTMLSelectElement).selectedOptions[0]?.text).toBe('Auto (cloud: native · local: XML)')
    expect((fieldByLabel(w, /guardrail timeout behavio/i).element as HTMLSelectElement).selectedOptions[0]?.text).toBe('Default (fail open)')
  })

  it('writes safety timeouts to the draft', async () => {
    const { w, draft } = mountTuning('cloud')
    await fieldByLabel(w, /per-tool timeout/i).setValue('90')
    await fieldByLabel(w, /timeout behavio/i).setValue('fail-closed')
    expect(draft.tool_timeout_seconds).toBe(90)
    expect(draft.guardrail_timeout_behavior).toBe('fail-closed')
  })

  describe('temperature', () => {
    it('shows "Provider default" as the cloud placeholder and explains blank / 0', () => {
      const { w } = mountTuning('cloud')
      expect(fieldByLabel(w, /temperature/i).attributes('placeholder')).toBe('Provider default')
      expect(w.text()).toContain("Blank or 0 uses the provider's default. Set 0.05–2 to override.")
    })

    it('shows "Server default" as the local placeholder, never a fabricated 0.1', () => {
      const { w } = mountTuning('local')
      expect(fieldByLabel(w, /temperature/i).attributes('placeholder')).toBe('Server default')
      expect(w.text()).toContain("Blank uses the server's default.")
      expect(w.text()).not.toContain("Blank or 0 uses the provider's default")
    })
  })

  it('says a blank cloud numeric field uses the provider default, but local keeps its wording', () => {
    const cloud = mountTuning('cloud').w.text()
    expect(cloud).toContain("Blank uses the provider's default.")
    expect(cloud).not.toContain('Blank or 0 uses the default')
    const local = mountTuning('local').w.text()
    expect(local).toContain('Blank or 0 uses the default')
    expect(local).not.toContain("Blank uses the provider's default.")
  })

  it('does not claim 0 turns a safety timeout off', () => {
    const cloud = mountTuning('cloud').w.text()
    expect(cloud).not.toMatch(/0 = off/i)
    expect(cloud).toMatch(/Safety timeouts · blank uses the provider's default/)
    expect(mountTuning('local').w.text()).toMatch(/Safety timeouts · blank or 0 = default/)
  })
})
