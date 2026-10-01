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
  config: { providers: {} },
} as unknown as AdminState)
vi.mock('../../../../composables/models/useModels', () => ({ useModels: () => ({ state: adminState }) }))

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
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('groups the form into Basics, Model & access, Schedule and Review', async () => {
    const w = await mountForm()
    expect(w.findAll('h2').map((h) => h.text())).toEqual(['Basics', 'Model & access', 'Schedule', 'Review'])
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
      { name: 'nightly', trigger: { type: 'manual', value: '' }, task_file: 'task.md', strategy: 'persistent', model: '', loop_strategy: '', network_grant: '' },
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
      { name: 'nightly-v2', trigger: { type: 'interval', value: '1h' }, task_file: 'task.md', strategy: 'persistent', model: 'qwen', loop_strategy: '', network_grant: '' },
    ]])
  })

  it('marks unsaved changes, and cancels', async () => {
    const w = await mountForm(AUTO)
    expect(w.text()).not.toContain('Unsaved changes')
    await control(w, 'Name').setValue('nightly-v2')
    expect(w.text()).toContain('Unsaved changes')
    await w.findAll('button').find((b) => b.text() === 'Cancel')!.trigger('click')
    expect(w.emitted('cancel')).toHaveLength(1)
  })
})
