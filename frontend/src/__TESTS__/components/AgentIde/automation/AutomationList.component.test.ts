import { describe, it, expect, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../../router'
import { useConfirm } from '../../../../composables/ui/useConfirm'
import AutomationList from '../../../../components/AgentIde/automation/AutomationList.vue'
import type { Automation } from '../../../../types/dispatcher'

const auto = (name: string, over: Partial<Automation> = {}): Automation =>
  ({ id: `ws/${name}`, workspace: 'ws', name, task_file: 't.md', strategy: 'persistent', trigger: 'interval', trigger_value: '1h', model: 'qwen', ...over })

const mounted: VueWrapper[] = []
async function mountList(automations: Automation[]) {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/automations')
  const w = mount(AutomationList, { props: { automations, loading: false }, global: { plugins: [router], stubs: { Icon: true, BrandMark: true } }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return w
}
const row = (w: VueWrapper, name: string) => w.findAll('tbody tr').find((r) => r.text().includes(name))!
const named = (w: VueWrapper, label: string) => w.find(`button[aria-label="${label}"]`)

describe('AutomationList', () => {
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('links each automation and shows its workspace, schedule, model and status', async () => {
    const w = await mountList([auto('nightly')])
    const r = row(w, 'nightly')
    expect(r.find('a').attributes('href')).toBe('/automations/ws%2Fnightly')
    expect(r.text()).toContain('ws')
    expect(r.text()).toContain('Every 1h')
    expect(r.text()).toContain('qwen')
    expect(r.text()).toContain('Idle')
  })

  // Carried over from AutomationsPanel: the backend's `queued` / `queue_position`
  // fields drive the badge; running wins over queued.
  it('shows the queue position from `queued`, and running over queued', async () => {
    const w = await mountList([auto('a', { queued: true, queue_position: 3 }), auto('b', { is_running: true, queued: true })])
    expect(row(w, 'a').text()).toContain('Queued #3')
    expect(row(w, 'b').text()).toContain('Running')
    expect(row(w, 'b').text()).not.toContain('Queued')
  })

  it('offers cancelling only a queued run, after confirming', async () => {
    const w = await mountList([auto('a', { queued: true, queue_position: 1 }), auto('b', { is_running: true })])
    expect(named(w, 'Cancel queued run of b').exists()).toBe(false)
    await named(w, 'Cancel queued run of a').trigger('click')
    await flushPromises()
    expect(w.emitted('cancel-queued')).toBeUndefined()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('cancel-queued')).toEqual([[expect.objectContaining({ name: 'a' })]])
  })

  it('runs or stops, and locks the rest of a busy workspace', async () => {
    const w = await mountList([auto('a', { is_running: true }), auto('b'), auto('c', { workspace: 'lab', id: 'lab/c' })])
    await named(w, 'Stop a').trigger('click')
    expect(w.emitted('stop')).toEqual([[expect.objectContaining({ name: 'a' })]])
    expect(named(w, 'Run b').attributes('disabled')).toBeDefined()
    expect(named(w, 'Delete b').attributes('disabled')).toBeDefined()
    expect(row(w, 'b').find('a[aria-label="Edit b"]').exists()).toBe(false)
    await named(w, 'Run c').trigger('click')
    expect(w.emitted('run')).toEqual([[expect.objectContaining({ name: 'c' })]])
  })

  it('deletes only after confirming', async () => {
    const w = await mountList([auto('a')])
    await named(w, 'Delete a').trigger('click')
    await flushPromises()
    expect(w.emitted('delete')).toBeUndefined()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete')).toEqual([[expect.objectContaining({ name: 'a' })]])
  })

  it('filters by workspace and search', async () => {
    const w = await mountList([auto('nightly'), auto('hourly', { workspace: 'lab', id: 'lab/hourly' })])
    await w.get('select[aria-label="Workspace"]').setValue('lab')
    expect(w.findAll('tbody tr')).toHaveLength(1)
    await w.get('select[aria-label="Workspace"]').setValue('')
    await w.get('input[type="search"]').setValue('night')
    expect(w.findAll('tbody tr').map((r) => r.text())).toEqual([expect.stringContaining('nightly')])
  })

  it('invites creating the first automation', async () => {
    const w = await mountList([])
    expect(w.text()).toContain('No automations yet')
    expect(w.find('a[href="/automations/new"]').exists()).toBe(true)
  })
})
