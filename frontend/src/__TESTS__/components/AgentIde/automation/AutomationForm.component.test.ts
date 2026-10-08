import { describe, it, expect, vi, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { computed, ref } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../../router'
import type { AdminState } from '../../../../types/admin'
import type { Automation } from '../../../../types/dispatcher'

vi.mock('../../../../composables/settings/useHostNetworkState', () => ({
  useHostNetworkState: () => ({ hostNetworkOff: computed(() => false), hostNetworkKnown: computed(() => true), load: vi.fn() }),
}))
const adminState = ref<AdminState | null>({
  models: [{ name: 'qwen', provider: 'local', endpoint: '', active: false, ready: false }],
  config: { providers: {}, communication: { connectors: { 'my-telegram': { type: 'telegram', enabled: true, settings: {} }, 'old-bot': { type: 'telegram', enabled: false, settings: {} } } } },
} as unknown as AdminState)
vi.mock('../../../../composables/models/useModels', () => ({ useModels: () => ({ state: adminState }) }))

const INITIAL_STATE = adminState.value

import AutomationForm from '../../../../components/AgentIde/automation/AutomationForm.vue'

const PROPS = { workspaces: [{ id: 'ws' }], workspaceFiles: { ws: ['jobs/nightly.md', 'task.md'] } }
const AUTO = { id: 'ws/nightly', workspace: 'ws', name: 'nightly', task_file: 'task.md', strategy: 'persistent', trigger: 'interval', trigger_value: '1h', model: 'qwen' } as Automation

const mounted: VueWrapper[] = []
async function mountForm(editAutomation: Automation | null = null) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(editAutomation ? '/automations/ws%2Fnightly/edit' : '/automations/new')
  const w = mount(AutomationForm, { props: { ...PROPS, editAutomation }, global: { plugins: [router], stubs: { Icon: true } }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return w
}
// Controls are found by their visible label, as a user would.
const control = (w: VueWrapper, label: string) => w.get(`#${w.findAll('label').find((l) => l.text() === label)!.attributes('for')}`)
const submitButton = (w: VueWrapper) => w.findAll('button').find((b) => /Create automation|Save changes/.test(b.text()))!

// Characterised against the old form first (plan D22), then updated to the
// grouped edit page: the create / update payloads are unchanged.
describe('AutomationForm', () => {
  afterEach(() => {
    adminState.value = INITIAL_STATE
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('groups the form into Basics, Model & access, Delivery, Schedule and Review', async () => {
    const w = await mountForm()
    expect(w.findAll('h2').map((h) => h.text())).toEqual(['Basics', 'Model & access', 'Delivery', 'Schedule', 'Review'])
  })

  it('warns that a scheduled run on the workspace default or a local model starts the local model', async () => {
    const w = await mountForm()
    await control(w, 'Workspace').setValue('ws')
    await w.findAll('[role="radio"]').find((r) => r.text() === 'Interval')!.trigger('click')
    expect(w.get('[data-test="wake-notice"]').text()).toMatch(/default/i)
    await control(w, 'Connection').setValue('local')
    expect(w.get('[data-test="wake-notice"]').text()).toMatch(/start the local model/i)
    await w.findAll('[role="radio"]').find((r) => r.text() === 'Manual')!.trigger('click')
    expect(w.find('[data-test="wake-notice"]').exists()).toBe(false)
  })

  it('offers nested task files of the chosen workspace', async () => {
    const w = await mountForm()
    await control(w, 'Workspace').setValue('ws')
    await flushPromises()
    const options = control(w, 'Task file').findAll('option').map((o) => o.attributes('value'))
    expect(options).toEqual(expect.arrayContaining(['jobs/nightly.md', 'task.md']))
  })

  it('sends the create payload once workspace, name, task file and trigger are set', async () => {
    const w = await mountForm()
    expect(submitButton(w).attributes('disabled')).toBeDefined()
    await control(w, 'Workspace').setValue('ws')
    await control(w, 'Name').setValue('nightly')
    await control(w, 'Task file').setValue('task.md')
    await w.findAll('[role="radio"]').find((r) => r.text() === 'Manual')!.trigger('click')
    expect(w.get('[data-test="review"]').text()).toContain('Manual only')
    await w.get('form').trigger('submit')
    expect(w.emitted('create-automation')).toEqual([[
      'ws',
      { name: 'nightly', trigger: { type: 'manual', value: '' }, task_file: 'task.md', strategy: 'persistent', model: '', loop_strategy: '', network_grant: '', memory_mode: '', notify: null, skip_if_busy: false, journal: false },
    ]])
  })

  it('explains a name the server would reject, and does not send it', async () => {
    const w = await mountForm(AUTO)
    await control(w, 'Name').setValue('bad name!')
    await w.get('form').trigger('submit')
    expect(w.get('[role="alert"]').text()).toContain('letters, digits, dashes and underscores')
    expect(w.emitted('update-automation')).toBeUndefined()
  })

  it('sends the update payload with the old name when editing, and locks the workspace', async () => {
    const w = await mountForm(AUTO)
    expect(control(w, 'Workspace').attributes('disabled')).toBeDefined()
    await control(w, 'Name').setValue('nightly-v2')
    await w.get('form').trigger('submit')
    expect(w.emitted('update-automation')).toEqual([[
      'ws',
      'nightly',
      { name: 'nightly-v2', trigger: { type: 'interval', value: '1h' }, task_file: 'task.md', strategy: 'persistent', model: 'qwen', loop_strategy: '', network_grant: '', memory_mode: '', notify: null, skip_if_busy: false, journal: false },
    ]])
  })

  it('lets the operator override the memory default per automation, and shows it in the review', async () => {
    const w = await mountForm(AUTO)
    const memory = () => w.get('[role="radiogroup"][aria-label="Memory"]').findAll('[role="radio"]')
    expect(memory().map((r) => r.text())).toEqual(['Default (off)', 'On', 'Off'])
    await memory()[1]!.trigger('click')
    expect(w.get('[data-test="review"]').text()).toContain('On')
    await w.get('form').trigger('submit')
    const [, , payload] = w.emitted('update-automation')![0] as [string, string, { memory_mode: string }]
    expect(payload.memory_mode).toBe('on')
  })

  it('names the live global default in the memory control', async () => {
    adminState.value = { ...adminState.value!, config: { ...adminState.value!.config, memory: { assistant_hot: true, automation_hot: true } } } as AdminState
    const w = await mountForm(AUTO)
    expect(w.get('[role="radiogroup"][aria-label="Memory"]').findAll('[role="radio"]')[0]!.text()).toBe('Default (on)')
  })

  it('marks unsaved changes, and cancels', async () => {
    const w = await mountForm(AUTO)
    expect(w.text()).not.toContain('Unsaved changes')
    await control(w, 'Name').setValue('nightly-v2')
    expect(w.text()).toContain('Unsaved changes')
    await w.findAll('button').find((b) => b.text() === 'Cancel')!.trigger('click')
    expect(w.emitted('cancel')).toHaveLength(1)
  })
  describe('delivery', () => {
    const submitUpdate = async (w: VueWrapper) => {
      await w.get('form').trigger('submit')
      return (w.emitted('update-automation')![0] as [string, string, { notify: unknown; skip_if_busy: boolean }])[2]
    }

    it('offers the configured connectors, marking disabled ones, and sends nothing by default', async () => {
      const w = await mountForm(AUTO)
      const options = control(w, 'Send results to').findAll('option').map((o) => [o.attributes('value'), o.text()])
      expect(options).toEqual([['', "Don't send results"], ['my-telegram', 'my-telegram'], ['old-bot', 'old-bot (disabled)']])
      expect(w.text()).not.toContain('Skip items already reported')
      expect((await submitUpdate(w)).notify).toBeNull()
    })

    it('reveals the delivery options once a connector is chosen and sends them', async () => {
      const w = await mountForm(AUTO)
      await control(w, 'Send results to').setValue('my-telegram')
      await w.findAll('input[role="switch"]').find((i) => i.element.closest('label')?.textContent?.includes('Skip items already reported'))!.setValue(true)
      await control(w, 'Remember reported items for (days)').setValue('30')
      expect(w.get('[data-test="review"]').text()).toContain('my-telegram · skips repeats')
      expect((await submitUpdate(w)).notify).toEqual({ connector: 'my-telegram', dedup: true, dedup_days: 30, send_empty: false })
    })

    it('rejects a retention that is not a whole number and does not send', async () => {
      const w = await mountForm(AUTO)
      await control(w, 'Send results to').setValue('my-telegram')
      await w.findAll('input[role="switch"]').find((i) => i.element.closest('label')?.textContent?.includes('Skip items already reported'))!.setValue(true)
      await control(w, 'Remember reported items for (days)').setValue('soon')
      expect(w.text()).toContain('Enter a whole number of days')
      expect(submitButton(w).attributes('disabled')).toBeDefined()
      await w.get('form').trigger('submit')
      expect(w.emitted('update-automation')).toBeUndefined()
    })

    it('keeps a connector that is no longer configured selectable when editing', async () => {
      const w = await mountForm({ ...AUTO, notify: { connector: 'gone', dedup: true } })
      expect(control(w, 'Send results to').findAll('option').map((o) => o.text())).toContain('gone (not configured)')
      expect((control(w, 'Send results to').element as HTMLSelectElement).value).toBe('gone')
    })

    it('does not claim there are no connectors while the admin state is still loading', async () => {
      const original = adminState.value
      adminState.value = null
      const w = await mountForm(AUTO)
      expect(w.text()).not.toContain('No connectors yet')
      adminState.value = original
    })

    it('points to Settings when no connector exists yet', async () => {
      const original = adminState.value
      adminState.value = { ...original!, config: { providers: {} } } as unknown as AdminState
      const w = await mountForm(AUTO)
      expect(w.text()).toContain('No connectors yet')
      expect(w.get('a[href*="communication"]').text()).toContain('Settings')
      adminState.value = original
    })
  })

  describe('skip when busy', () => {
    it('is offered for scheduled runs only, and sent when switched on', async () => {
      const w = await mountForm(AUTO)
      const toggle = () => w.findAll('input[role="switch"]').find((i) => i.element.closest('label')?.textContent?.includes('Skip a run if the model is busy'))
      await toggle()!.setValue(true)
      expect(w.get('[data-test="review"]').text()).toContain('Skips the run')
      await w.get('form').trigger('submit')
      expect((w.emitted('update-automation')![0] as unknown[])[2]).toMatchObject({ skip_if_busy: true })

      await w.findAll('[role="radio"]').find((r) => r.text() === 'Manual')!.trigger('click')
      expect(toggle()).toBeUndefined()
      expect(w.get('[data-test="review"]').text()).toContain('Waits its turn')
    })

    it('never sends skip_if_busy for a manual automation, even if it was switched on before', async () => {
      const w = await mountForm({ ...AUTO, skip_if_busy: true })
      await w.findAll('[role="radio"]').find((r) => r.text() === 'Manual')!.trigger('click')
      await w.get('form').trigger('submit')
      expect((w.emitted('update-automation')![0] as unknown[])[2]).toMatchObject({ skip_if_busy: false })
    })
  })

  describe('learning journal', () => {
    const toggle = (w: VueWrapper) => w.findAll('input[role="switch"]').find((i) => i.element.closest('label')?.textContent?.includes('Keep a journal between runs'))!

    it('is off by default and sent when switched on', async () => {
      const w = await mountForm(AUTO)
      expect((toggle(w).element as HTMLInputElement).checked).toBe(false)
      expect(w.get('[data-test="review"]').text()).toContain('Journal')
      await toggle(w).setValue(true)
      await w.get('form').trigger('submit')
      expect((w.emitted('update-automation')![0] as unknown[])[2]).toMatchObject({ journal: true })
    })

    it('shows an automation that already keeps a journal as on', async () => {
      const w = await mountForm({ ...AUTO, journal: true })
      expect((toggle(w).element as HTMLInputElement).checked).toBe(true)
    })
  })
})
