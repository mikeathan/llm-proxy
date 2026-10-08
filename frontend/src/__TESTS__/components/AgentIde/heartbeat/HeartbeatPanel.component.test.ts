import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../../router'
import type { HeartbeatState } from '../../../../types/heartbeat'

const { getHeartbeat, putHeartbeat } = vi.hoisted(() => ({ getHeartbeat: vi.fn(), putHeartbeat: vi.fn() }))
vi.mock('../../../../services/automation/dispatcherService', () => ({
  DispatcherService: { getHeartbeat, putHeartbeat },
}))
vi.mock('../../../../composables/models/useModels', () => ({
  useModels: () => ({
    state: ref({
      models: [{ name: 'qwen', provider: 'local' }, { name: 'gpt-5', provider: 'openai' }],
      config: { communication: { connectors: { 'my-telegram': { type: 'telegram', enabled: true, settings: {} } } } },
    }),
  }),
}))

import HeartbeatPanel from '../../../../components/AgentIde/heartbeat/HeartbeatPanel.vue'

const STATE: HeartbeatState = {
  config: { enabled: false },
  lane: 'cloud',
  wakes_local_model: false,
  has_checks: true,
}
const stateWith = (over: Partial<HeartbeatState>): HeartbeatState => ({ ...STATE, ...over })

const mounted: VueWrapper[] = []
async function mountPanel(state: HeartbeatState = STATE) {
  getHeartbeat.mockResolvedValue(state)
  const router = createAppRouter(createMemoryHistory())
  await router.push('/workspaces/demo/heartbeat')
  const w = mount(HeartbeatPanel, { props: { workspaceId: 'demo' }, global: { plugins: [router], stubs: { Icon: true } }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return w
}
const control = (w: VueWrapper, label: string) => w.get(`#${w.findAll('label').find((l) => l.text() === label)!.attributes('for')}`)
const toggle = (w: VueWrapper) => w.get('input[role="switch"]')
const saveButton = (w: VueWrapper) => w.findAll('button').find((b) => b.text() === 'Save')!

describe('HeartbeatPanel', () => {
  beforeEach(() => {
    getHeartbeat.mockReset()
    putHeartbeat.mockReset().mockImplementation(async (_ws: string, config: HeartbeatState['config']) => stateWith({ config }))
  })
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('starts off, with the settings greyed out until it is switched on', async () => {
    const w = await mountPanel()
    expect((toggle(w).element as HTMLInputElement).checked).toBe(false)
    expect(control(w, 'Check every').attributes('disabled')).toBeDefined()
    await toggle(w).setValue(true)
    expect(control(w, 'Check every').attributes('disabled')).toBeUndefined()
  })

  it('saves what was set, only once something changed, and shows what the server kept', async () => {
    const w = await mountPanel()
    expect(saveButton(w).attributes('disabled')).toBeDefined()
    await toggle(w).setValue(true)
    await control(w, 'Check every').setValue('1h')
    await control(w, 'Model').setValue('gpt-5')
    await control(w, 'Send alerts to').setValue('my-telegram')
    expect(w.text()).toContain('Unsaved changes')
    await saveButton(w).trigger('click')
    await flushPromises()
    expect(putHeartbeat).toHaveBeenCalledWith('demo', { enabled: true, every: '1h', model: 'gpt-5', notify: { connector: 'my-telegram' } })
    expect(w.text()).not.toContain('Unsaved changes')
  })

  it('keeps the delivery details already saved when only the connector stays the same', async () => {
    const state = stateWith({ config: { enabled: true, every: '30m', notify: { connector: 'my-telegram', dedup: true, dedup_days: 14 } } })
    const w = await mountPanel(state)
    await control(w, 'Check every').setValue('2h')
    await saveButton(w).trigger('click')
    await flushPromises()
    expect(putHeartbeat.mock.lastCall![1].notify).toEqual({ connector: 'my-telegram', dedup: true, dedup_days: 14 })
  })

  it('warns that every check wakes the local model, only when it would', async () => {
    const local = await mountPanel(stateWith({ lane: 'local', wakes_local_model: true, config: { enabled: true } }))
    expect(local.get('[role="note"]').text()).toContain('starts the local model')
    const cloud = await mountPanel(stateWith({ config: { enabled: true, model: 'gpt-5' } }))
    expect(cloud.findAll('[role="note"]').map((n) => n.text()).join(' ')).not.toContain('local model')
  })

  it('says there is nothing to check yet, and links to heartbeat.md', async () => {
    const w = await mountPanel(stateWith({ has_checks: false, config: { enabled: true } }))
    expect(w.text()).toContain('No checks yet')
    expect(w.get('a[href="/workspaces/demo/files/heartbeat.md"]').text()).toContain('heartbeat.md')
    const ready = await mountPanel()
    expect(ready.text()).not.toContain('No checks yet')
  })

  it.each([
    [undefined, /no checks have run yet/i],
    [{ at: '2026-10-04T14:30:00Z', result: 'quiet' as const }, /nothing to report/i],
    [{ at: '2026-10-04T14:30:00Z', result: 'alert' as const }, /alert/i],
    [{ at: '2026-10-04T14:30:00Z', result: 'skipped_busy' as const }, /busy/i],
    [{ at: '2026-10-04T14:30:00Z', result: 'error' as const }, /failed/i],
  ])('shows the last check: %o', async (status, words) => {
    const w = await mountPanel(stateWith({ status, config: { enabled: true } }))
    expect(w.get('[data-test="heartbeat-status"]').text()).toMatch(words)
  })

  it('says why a save failed and keeps the edits', async () => {
    putHeartbeat.mockRejectedValueOnce(new Error('invalid heartbeat.every "5s"'))
    const w = await mountPanel()
    await toggle(w).setValue(true)
    await saveButton(w).trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('invalid heartbeat.every')
    expect((toggle(w).element as HTMLInputElement).checked).toBe(true)
    expect(w.text()).toContain('Unsaved changes')
  })

  it('says it could not load, and retries', async () => {
    getHeartbeat.mockRejectedValueOnce(new Error('boom'))
    const router = createAppRouter(createMemoryHistory())
    await router.push('/workspaces/demo/heartbeat')
    const w = mount(HeartbeatPanel, { props: { workspaceId: 'demo' }, global: { plugins: [router], stubs: { Icon: true } }, attachTo: document.body })
    mounted.push(w)
    await flushPromises()
    expect(w.text()).toContain('boom')
    getHeartbeat.mockResolvedValue(STATE)
    await w.findAll('button').find((b) => b.text() === 'Retry')!.trigger('click')
    await flushPromises()
    expect(w.find('input[role="switch"]').exists()).toBe(true)
  })

  it('discards the edits', async () => {
    const w = await mountPanel()
    await toggle(w).setValue(true)
    await w.findAll('button').find((b) => b.text() === 'Discard')!.trigger('click')
    expect((toggle(w).element as HTMLInputElement).checked).toBe(false)
  })
})
