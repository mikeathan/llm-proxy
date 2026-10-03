import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GlobalSettings from '../../../components/settings/GlobalSettings.vue'
import { DEFAULT_CONFIG } from '../../../composables/models/useConfig'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { GlobalConfig } from '../../../types/admin'
import type { Model } from '../../../types/model'

const MODELS = [{ name: 'qwen', provider: 'local' }, { name: 'gpt', provider: 'openai' }] as Model[]

// VTU's emitted() does not record script-setup emits in this harness, so the
// listener callbacks are the component's observable contract.
function mountGlobal(overrides: Partial<GlobalConfig> = {}) {
  const onUpdate = vi.fn()
  const onSave = vi.fn()
  const onLogLevel = vi.fn()
  const w = mount(GlobalSettings, {
    props: { editConfig: { ...structuredClone(DEFAULT_CONFIG), ...overrides }, logLevel: 'info', models: MODELS },
    attrs: { 'onUpdate:editConfig': onUpdate, onUpdateConfig: onSave, onUpdateLogLevel: onLogLevel },
    global: { stubs: { Icon: true } },
  })
  const last = (): GlobalConfig => onUpdate.mock.lastCall![0]
  return { w, onUpdate, onSave, onLogLevel, last }
}

const SCHEDULER_DEFAULTS = {
  local_concurrency: 1,
  cloud_concurrency: 3,
  preempt_automations: true,
  inbound_wait_seconds: 60,
  inbound_wait_by_default: false,
  inbound_max_queued: 32,
  inbound_preempt: false,
}

describe('GlobalSettings', () => {
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

  it('writes the memory defaults as a pair, starting from assistant on and automations off', async () => {
    // The parent owns the config, so each interaction starts from the props as mounted.
    const { w, last } = mountGlobal()
    expect(fieldByLabel(w, /assistant remembers/i).element).toHaveProperty('checked', true)
    expect(fieldByLabel(w, /automations remember/i).element).toHaveProperty('checked', false)
    await fieldByLabel(w, /assistant remembers/i).setValue(false)
    expect(last().memory).toEqual({ assistant_hot: false, automation_hot: false })
    await fieldByLabel(w, /automations remember/i).setValue(true)
    expect(last().memory).toEqual({ assistant_hot: true, automation_hot: true })
  })

  it('clamps the inbound wait at -1 (no limit)', async () => {
    const { w, last } = mountGlobal()
    await fieldByLabel(w, /wait up to/i).setValue('-5')
    expect(last().scheduler?.inbound_wait_seconds).toBe(-1)
  })

  it('splits default arguments into an argument list', async () => {
    const { w, last } = mountGlobal()
    await fieldByLabel(w, /default arguments/i).setValue('--ctx-size 4096 --flash-attn')
    expect(last().providers?.local?.default_args).toEqual(['--ctx-size', '4096', '--flash-attn'])
  })

  it('commits environment variables on blur, not on every keystroke', async () => {
    const { w, onUpdate, last } = mountGlobal()
    const env = fieldByLabel(w, /environment variables/i)
    await env.setValue('A=1\nB=2')
    expect(onUpdate).not.toHaveBeenCalled()
    await env.trigger('blur')
    expect(last().providers?.local?.environment).toEqual({ A: '1', B: '2' })
  })

  it('shows only the GPU fields the chosen provider uses', () => {
    const labels = (gpu: string) => mountGlobal({ gpu_provider: gpu }).w.findAll('label').map((l) => l.text()).join('|')
    expect(labels('macos')).not.toMatch(/gpu index/i)
    expect(labels('nvidia')).toMatch(/gpu index/i)
    expect(labels('nvidia')).toMatch(/tool binary/i)
    expect(labels('sysfs')).toMatch(/sysfs device path/i)
    expect(labels('sysfs')).not.toMatch(/tool binary/i)
  })

  it('turns run logging on', async () => {
    const { w, last } = mountGlobal({ run_logging: { enabled: false } })
    await fieldByLabel(w, /run logging/i).setValue(true)
    expect(last().run_logging?.enabled).toBe(true)
  })

  it('changes the log level on the spot', async () => {
    const { w, onLogLevel } = mountGlobal()
    await w.findAll('button').find((b) => /^debug$/i.test(b.text()))!.trigger('click')
    expect(onLogLevel).toHaveBeenCalledWith('DEBUG')
  })

  it('asks the page to save on submit', async () => {
    const { w, onSave } = mountGlobal()
    await w.find('form').trigger('submit')
    expect(onSave).toHaveBeenCalledTimes(1)
  })
})
