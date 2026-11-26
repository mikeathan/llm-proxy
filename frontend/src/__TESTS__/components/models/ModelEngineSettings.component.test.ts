import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelEngineSettings from '../../../components/models/ModelEngineSettings.vue'
import { DEFAULT_CONFIG } from '../../../composables/models/useConfig'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { GlobalConfig } from '../../../types/admin'

// Models › Engine (moved from Settings · General, ADR-0090).
function mountEngine(overrides: Partial<GlobalConfig> = {}) {
  const onUpdate = vi.fn()
  const w = mount(ModelEngineSettings, {
    props: { editConfig: { ...structuredClone(DEFAULT_CONFIG), ...overrides } },
    attrs: { 'onUpdate:editConfig': onUpdate },
    global: { stubs: { Icon: true } },
  })
  return { w, onUpdate, last: (): GlobalConfig => onUpdate.mock.lastCall![0] }
}

describe('ModelEngineSettings', () => {
  it('splits default arguments into an argument list', async () => {
    const { w, last } = mountEngine()
    await fieldByLabel(w, /default arguments/i).setValue('--ctx-size 4096 --flash-attn')
    expect(last().providers?.local?.default_args).toEqual(['--ctx-size', '4096', '--flash-attn'])
  })

  it('commits environment variables on blur, not on every keystroke', async () => {
    const { w, onUpdate, last } = mountEngine()
    const env = fieldByLabel(w, /environment variables/i)
    await env.setValue('A=1\nB=2')
    expect(onUpdate).not.toHaveBeenCalled()
    await env.trigger('blur')
    expect(last().providers?.local?.environment).toEqual({ A: '1', B: '2' })
  })

  it('shows only the GPU fields the chosen provider uses', () => {
    const labels = (gpu: string) => mountEngine({ gpu_provider: gpu }).w.findAll('label').map((l) => l.text()).join('|')
    expect(labels('macos')).not.toMatch(/gpu index/i)
    expect(labels('nvidia')).toMatch(/gpu index/i)
    expect(labels('nvidia')).toMatch(/tool binary/i)
    expect(labels('sysfs')).toMatch(/sysfs device path/i)
    expect(labels('sysfs')).not.toMatch(/tool binary/i)
  })

  it('shows an unset GPU provider as None, which is what the server does with it', () => {
    const { w } = mountEngine({ gpu_provider: undefined })
    expect((fieldByLabel(w, /gpu provider/i).element as HTMLSelectElement).selectedOptions[0]?.text).toBe('None — no GPU metrics')
  })

  it('saves the idle timeout and the model directory', async () => {
    const { w, last } = mountEngine()
    await fieldByLabel(w, /idle timeout/i).setValue('-1')
    expect(last().idle_timeout_seconds).toBe(-1)
    await fieldByLabel(w, /model directory/i).setValue('/srv/models')
    expect(last().providers?.local?.model_dir).toBe('/srv/models')
  })
})
