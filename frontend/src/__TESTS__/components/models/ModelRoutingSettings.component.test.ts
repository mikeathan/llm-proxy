import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelRoutingSettings from '../../../components/models/ModelRoutingSettings.vue'
import { DEFAULT_CONFIG } from '../../../composables/models/useConfig'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { GlobalConfig } from '../../../types/admin'
import type { Model } from '../../../types/model'

// Models › Routing (moved from Settings · General, ADR-0090). VTU's emitted() does not record script-setup
// emits in this harness, so the listener callbacks are the component's observable contract.
const MODELS = [
  { name: 'qwen', provider: 'local', workload_class: 'local' },
  { name: 'gpt', provider: 'openai', workload_class: 'cloud' },
] as Model[]

function mountGlobal(overrides: Partial<GlobalConfig> = {}) {
  const onUpdate = vi.fn()
  const w = mount(ModelRoutingSettings, {
    props: { editConfig: { ...structuredClone(DEFAULT_CONFIG), ...overrides }, models: MODELS },
    attrs: { 'onUpdate:editConfig': onUpdate },
    global: { stubs: { Icon: true } },
  })
  const last = (): GlobalConfig => onUpdate.mock.lastCall![0]
  return { w, onUpdate, last }
}
const mountHelpers = mountGlobal

const SCHEDULER_DEFAULTS = {
  local_concurrency: 1,
  cloud_concurrency: 3,
  preempt_automations: true,
  inbound_wait_seconds: 60,
  inbound_wait_by_default: false,
  inbound_max_queued: 32,
  inbound_preempt: false,
}

describe('ModelRoutingSettings', () => {
  it('offers every configured model for primary and fallback routing', () => {
    const { w } = mountGlobal()
    const primary = fieldByLabel(w, /primary/i)
    expect(primary.findAll('option').map((o) => o.attributes('value'))).toEqual(['', 'qwen', 'gpt'])
    expect(fieldByLabel(w, /fallback/i).findAll('option')).toHaveLength(3)
  })

  it('writes the scheduler whole, seeded with defaults, and clamps counts to at least one', async () => {
    const { w, last } = mountGlobal()
    await fieldByLabel(w, /local runs/i).setValue('0')
    expect(last().scheduler).toEqual({ ...SCHEDULER_DEFAULTS, local_concurrency: 1 })
    await fieldByLabel(w, /cloud runs/i).setValue('5')
    expect(last().scheduler?.cloud_concurrency).toBe(5)
  })

  it('clamps the inbound wait at -1 (no limit)', async () => {
    const { w, last } = mountGlobal()
    await fieldByLabel(w, /wait up to/i).setValue('-5')
    expect(last().scheduler?.inbound_wait_seconds).toBe(-1)
  })
})

// A tool result (a file, a page, search hits) keeps more text when the window has room, and never less than 3000 characters:
// the switch is on unless the saved config says otherwise, and turning it off is saved as an explicit false.
describe('ModelRoutingSettings tool result size', () => {
  const toggle = (w: ReturnType<typeof mountGlobal>['w']) =>
    w.findAll('input[type="checkbox"]').find((t) => /tool results/i.test(t.element.closest('label')?.textContent ?? ''))!

  it('is on when the config says nothing', () => {
    const { w } = mountGlobal({})
    expect((toggle(w).element as HTMLInputElement).checked).toBe(true)
    expect(w.text()).toContain('never less than 3000 characters')
  })

  it('reflects a saved off', () => {
    const { w } = mountGlobal({ agent: { adaptive_result_cap: false } })
    expect((toggle(w).element as HTMLInputElement).checked).toBe(false)
  })

  it('saves an explicit false and an explicit true', async () => {
    // The switch shows what the parent holds, so each direction starts from its own mount.
    const on = mountGlobal({})
    await toggle(on.w).setValue(false)
    expect(on.last().agent).toEqual({ adaptive_result_cap: false })
    const off = mountGlobal({ agent: { adaptive_result_cap: false } })
    await toggle(off.w).setValue(true)
    expect(off.last().agent).toEqual({ adaptive_result_cap: true })
  })
})

// Helpers (sub-agents) run on a cloud model only, so the picker offers cloud models and "Off"; the pool size rides
// along in the same agent block, keeping the block's other settings.
describe('ModelRoutingSettings helpers', () => {
  it('offers only cloud models, and off by default', () => {
    const { w } = mountHelpers()
    const select = fieldByLabel(w, /Helper model/)
    const options = select.findAll('option').map((o) => o.text())
    expect(options.some((o) => o.startsWith('gpt'))).toBe(true)
    expect(options.some((o) => o.startsWith('qwen'))).toBe(false)
    expect((select.element as HTMLSelectElement).value).toBe('')
  })

  // A Helper model removed from the models (or one that is not a cloud model) still shows, flagged, so the user sees
  // what is set and what to change, not a blank box.
  it('shows a helper model that is no longer available, flagged, and keeps it selected', () => {
    const { w } = mountHelpers({ agent: { subagent_model: 'gone-m' } })
    const select = fieldByLabel(w, /Helper model/)
    const options = select.findAll('option').map((o) => o.text())
    expect(options).toContain('gone-m (not available)')
    expect((select.element as HTMLSelectElement).value).toBe('gone-m')
  })

  it('shows no flagged option when the helper model is available or off', () => {
    expect(fieldByLabel(mountHelpers({ agent: { subagent_model: 'gpt' } }).w, /Helper model/).findAll('option').map((o) => o.text()).join('|')).not.toContain('not available')
    expect(fieldByLabel(mountHelpers().w, /Helper model/).findAll('option').map((o) => o.text()).join('|')).not.toContain('not available')
  })

  it('saves the helper model and pool size beside the other agent settings', async () => {
    const { w, last } = mountHelpers({ agent: { adaptive_result_cap: false } })
    await fieldByLabel(w, /Helper model/).setValue('gpt')
    expect(last().agent).toEqual({ adaptive_result_cap: false, subagent_model: 'gpt' })
    await fieldByLabel(w, /Helpers at once/).setValue('2')
    expect(last().agent?.subagent_concurrency).toBe(2)
  })
})
