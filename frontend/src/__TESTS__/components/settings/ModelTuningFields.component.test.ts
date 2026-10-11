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
  it('derives the context budget and output cap for local models (read-only)', () => {
    const { w } = mountTuning('local')
    expect(fieldByLabel(w, /context budget/i).attributes('readonly')).toBeDefined()
    expect(fieldByLabel(w, /max tokens/i).attributes('readonly')).toBeDefined()
    expect(w.text()).toMatch(/derived/i)
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
