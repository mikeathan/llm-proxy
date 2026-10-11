import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
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
// The picker asks the server to preview a schedule.
const previewMock = vi.hoisted(() => vi.fn())
vi.mock('../../../../services/automation/dispatcherService', () => ({
  DispatcherService: { previewSchedule: previewMock },
}))
// Saving a written task goes through the shared file writer; playbooks come from the template service.
const { writeFileMock, listTemplatesMock, getTemplateMock, confirmMock } = vi.hoisted(() => ({ writeFileMock: vi.fn(), listTemplatesMock: vi.fn(), getTemplateMock: vi.fn(), confirmMock: vi.fn() }))
vi.mock('../../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))
vi.mock('../../../../composables/editor/useWorkspaceFileWriter', () => ({ useWorkspaceFileWriter: () => ({ writeFile: writeFileMock }) }))
vi.mock('../../../../services/template/templateService', () => ({ TemplateService: { listTemplates: listTemplatesMock, getTemplate: getTemplateMock } }))
vi.mock('../../../../composables/models/useModels', () => ({ useModels: () => ({ state: adminState }) }))

const INITIAL_STATE = adminState.value

import AutomationForm from '../../../../components/AgentIde/automation/AutomationForm.vue'

const PROPS = { workspaces: [{ id: 'ws' }], workspaceFiles: { ws: ['jobs/nightly.md', 'task.md'] } }
const AUTO = { id: 'ws/nightly', workspace: 'ws', name: 'nightly', task_file: 'task.md', strategy: 'persistent', trigger: 'interval', trigger_value: '1h', model: 'qwen' } as Automation

const mounted: VueWrapper[] = []
async function mountForm(editAutomation: Automation | null = null, automations: Automation[] = [], start?: { workspace?: string; playbook?: string }) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(editAutomation ? '/automations/ws%2Fnightly/edit' : '/automations/new')
  const w = mount(AutomationForm, { props: { ...PROPS, editAutomation, automations, start }, global: { plugins: [router], stubs: { Icon: true } }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return w
}
// Controls are found by their visible label, as a user would.
const control = (w: VueWrapper, label: string) => w.get(`#${w.findAll('label').find((l) => l.text() === label)!.attributes('for')}`)
const chooseSchedule = (w: VueWrapper, kind: string) => w.get('[data-test="kind"]').setValue(kind)
const submitButton = (w: VueWrapper) => w.findAll('button').find((b) => /Create automation|Save changes/.test(b.text()))!

// Characterised against the old form first (plan D22), then updated to the
// grouped edit page: the create / update payloads are unchanged.
describe('AutomationForm', () => {
  beforeEach(() => {
    previewMock.mockReset().mockResolvedValue({ valid: true, timezone: 'UTC', next_runs: [] })
    confirmMock.mockReset().mockResolvedValue(true)
    writeFileMock.mockReset().mockResolvedValue('created')
    listTemplatesMock.mockReset().mockResolvedValue([{ id: 'brief', name: 'Brief', category: 'research' }])
    getTemplateMock.mockReset().mockResolvedValue({ id: 'brief', name: 'Brief', category: 'research', description: '', content: 'BRIEF TEXT' })
  })

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
    await chooseSchedule(w, 'every')
    expect(w.get('[data-test="wake-notice"]').text()).toMatch(/default/i)
    await control(w, 'Connection').setValue('local')
    expect(w.get('[data-test="wake-notice"]').text()).toMatch(/start the local model/i)
    await chooseSchedule(w, 'manual')
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
    await chooseSchedule(w, 'manual')
    expect(w.get('[data-test="review"]').text()).toContain('Manual only')
    await w.get('form').trigger('submit')
    expect(w.emitted('create-automation')).toEqual([[
      'ws',
      { name: 'nightly', trigger: { type: 'manual', value: '' }, task_file: 'task.md', strategy: 'persistent', model: '', loop_strategy: '', network_grant: '', memory_mode: '', notify: null, skip_if_busy: false, journal: false },
    ]])
  })

  it('creates a weekday schedule from the picker without anyone writing cron', async () => {
    const w = await mountForm()
    await control(w, 'Workspace').setValue('ws')
    await control(w, 'Name').setValue('morning')
    await control(w, 'Task file').setValue('task.md')
    expect(submitButton(w).attributes('disabled')).toBeUndefined() // a new automation starts manual
    await chooseSchedule(w, 'weekdays')
    await w.get('input[type="time"]').setValue('08:30')
    expect(w.get('[data-test="review"]').text()).toContain('Every weekday at 08:30')
    await w.get('form').trigger('submit')
    expect((w.emitted('create-automation')![0] as unknown[])[1]).toMatchObject({ trigger: { type: 'cron', value: '30 8 * * 1-5' } })
  })

  it('will not save while the server says the schedule is not valid', async () => {
    previewMock.mockResolvedValue({ valid: false, error: 'invalid cron expression', timezone: 'UTC', next_runs: [] })
    vi.useFakeTimers()
    try {
      const w = await mountForm()
      await control(w, 'Workspace').setValue('ws')
      await control(w, 'Name').setValue('nightly')
      await control(w, 'Task file').setValue('task.md')
      await chooseSchedule(w, 'custom')
      await w.get('[data-test="custom"]').setValue('*/10 * * * * *')
      await vi.advanceTimersByTimeAsync(400)
      await flushPromises()
      expect(submitButton(w).attributes('disabled')).toBeDefined()
      expect(w.text()).toContain('The schedule is not valid.')
      await w.get('[data-test="custom"]').setValue('*/10 * * * *')
      previewMock.mockResolvedValue({ valid: true, timezone: 'UTC', next_runs: [] })
      await vi.advanceTimersByTimeAsync(400)
      await flushPromises()
      expect(submitButton(w).attributes('disabled')).toBeUndefined()
    } finally {
      vi.useRealTimers()
    }
  })

  it('will not save a weekly schedule with no day chosen', async () => {
    const w = await mountForm()
    await control(w, 'Workspace').setValue('ws')
    await control(w, 'Name').setValue('weekly')
    await control(w, 'Task file').setValue('task.md')
    await chooseSchedule(w, 'weekly')
    await w.findAll('input[type="checkbox"]').find((c) => c.attributes('aria-label') === 'Monday')!.setValue(false)
    expect(submitButton(w).attributes('disabled')).toBeDefined()
  })

  describe('writing the task in the form', () => {
    const newTask = (w: VueWrapper) => w.findAll('[role="radio"]').find((r) => r.text() === 'New task')!.trigger('click')
    const fillBasics = async (w: VueWrapper, name = 'morning') => {
      await control(w, 'Workspace').setValue('ws')
      await control(w, 'Name').setValue(name)
    }

    it('saves the text as <name>.md and creates the automation on that file', async () => {
      const w = await mountForm()
      await fillBasics(w)
      await newTask(w)
      await control(w, 'Task').setValue('Summarise the news.')
      expect(w.get('[data-test="review"]').text()).toContain('morning.md')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(writeFileMock).toHaveBeenCalledWith('ws', 'morning.md', 'Summarise the news.')
      expect((w.emitted('create-automation')![0] as unknown[])[1]).toMatchObject({ name: 'morning', task_file: 'morning.md' })
      expect(w.emitted('fetch-files')!.some((e) => e[0] === 'ws')).toBe(true)
    })

    it('starts from a playbook the user picks, and lets them edit the text', async () => {
      const w = await mountForm()
      await fillBasics(w)
      await newTask(w)
      await flushPromises()
      expect(listTemplatesMock).toHaveBeenCalled()
      await control(w, 'Start from a playbook').setValue('brief')
      await flushPromises()
      expect((control(w, 'Task').element as HTMLTextAreaElement).value).toBe('BRIEF TEXT')
      await control(w, 'Task').setValue('BRIEF TEXT for Tuesdays')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(writeFileMock).toHaveBeenCalledWith('ws', 'morning.md', 'BRIEF TEXT for Tuesdays')
    })

    it('keeps what the user wrote, and the playbook list back at its prompt, when replacing is declined', async () => {
      confirmMock.mockResolvedValue(false)
      const w = await mountForm()
      await fillBasics(w)
      await newTask(w)
      await flushPromises()
      await control(w, 'Task').setValue('my own task')
      await control(w, 'Start from a playbook').setValue('brief')
      await flushPromises()
      expect((control(w, 'Task').element as HTMLTextAreaElement).value).toBe('my own task')
      expect((control(w, 'Start from a playbook').element as HTMLSelectElement).value).toBe('')
    })

    it('will not save until the task has some text', async () => {
      const w = await mountForm()
      await fillBasics(w)
      await newTask(w)
      expect(submitButton(w).attributes('disabled')).toBeDefined()
      expect(w.text()).toContain('Write the task.')
    })

    it('creates once when the button is pressed twice while the file is being saved', async () => {
      let finish: (outcome: string) => void = () => {}
      writeFileMock.mockImplementation(() => new Promise((resolve) => (finish = resolve)))
      const w = await mountForm()
      await fillBasics(w)
      await newTask(w)
      await control(w, 'Task').setValue('text')
      await w.get('form').trigger('submit')
      await w.get('form').trigger('submit')
      finish('created')
      await flushPromises()
      expect(writeFileMock).toHaveBeenCalledTimes(1)
      expect(w.emitted('create-automation')).toHaveLength(1)
    })

    it('creates nothing when replacing an existing file is declined', async () => {
      writeFileMock.mockResolvedValue('cancelled')
      const w = await mountForm()
      await fillBasics(w)
      await newTask(w)
      await control(w, 'Task').setValue('text')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(w.emitted('create-automation')).toBeUndefined()
    })

    it('creates nothing when the file cannot be written', async () => {
      writeFileMock.mockRejectedValue(new Error('disk full'))
      const w = await mountForm()
      await fillBasics(w)
      await newTask(w)
      await control(w, 'Task').setValue('text')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(w.emitted('create-automation')).toBeUndefined()
    })

    it('still creates from an existing file, writing nothing', async () => {
      const w = await mountForm()
      await fillBasics(w)
      await control(w, 'Task file').setValue('task.md')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(writeFileMock).not.toHaveBeenCalled()
      expect((w.emitted('create-automation')![0] as unknown[])[1]).toMatchObject({ task_file: 'task.md' })
    })

    it('is offered when creating, not when editing', async () => {
      const w = await mountForm(AUTO)
      expect(w.findAll('[role="radio"]').some((r) => r.text() === 'New task')).toBe(false)
    })

    // A saved task file may belong to another automation, and an automation name is unique per workspace:
    // nothing is written until both are clear, or a failed create would leave another automation running new text.
    it('writes nothing when the name is already taken in that workspace', async () => {
      const w = await mountForm(null, [{ ...AUTO, name: 'morning' }])
      await fillBasics(w)
      await newTask(w)
      await control(w, 'Task').setValue('text')
      expect(submitButton(w).attributes('disabled')).toBeDefined()
      expect(w.text()).toContain('An automation named morning already exists in this workspace.')
      // Shown on the field the user just typed in, not only in the footer.
      expect(control(w, 'Name').attributes('aria-invalid')).toBe('true')
      expect(w.get('[role="alert"]').text()).toContain('already exists')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(writeFileMock).not.toHaveBeenCalled()
      expect(w.emitted('create-automation')).toBeUndefined()
    })

    it('does not overwrite the task file another automation runs', async () => {
      const w = await mountForm(null, [{ ...AUTO, name: 'other', task_file: 'morning.md' }])
      await fillBasics(w)
      await newTask(w)
      await control(w, 'Task').setValue('text')
      expect(submitButton(w).attributes('disabled')).toBeDefined()
      expect(w.text()).toContain('morning.md is already the task of "other"')
      expect(writeFileMock).not.toHaveBeenCalled()
    })

    it('allows the same name in a different workspace', async () => {
      const w = await mountForm(null, [{ ...AUTO, workspace: 'elsewhere', name: 'morning' }])
      await fillBasics(w)
      await newTask(w)
      await control(w, 'Task').setValue('text')
      expect(submitButton(w).attributes('disabled')).toBeUndefined()
    })

    // The view keeps one form instance across new and edit.
    it('does not write a task file when an automation is opened for edit after New task was chosen', async () => {
      const w = await mountForm()
      await fillBasics(w)
      await newTask(w)
      await control(w, 'Task').setValue('text')
      await w.setProps({ editAutomation: AUTO })
      await flushPromises()
      expect(w.findAll('[role="radio"]').some((r) => r.text() === 'New task')).toBe(false)
      expect(control(w, 'Task file').element.tagName).toBe('SELECT')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(writeFileMock).not.toHaveBeenCalled()
      expect((w.emitted('update-automation')![0] as unknown[])[2]).toMatchObject({ task_file: 'task.md' })
    })

    it('opens already set up from a playbook: workspace chosen, task filled, named after it', async () => {
      const w = await mountForm(null, [], { workspace: 'ws', playbook: 'brief' })
      await flushPromises()
      expect((control(w, 'Workspace').element as HTMLSelectElement).value).toBe('ws')
      expect((control(w, 'Name').element as HTMLInputElement).value).toBe('brief')
      expect((control(w, 'Task').element as HTMLTextAreaElement).value).toBe('BRIEF TEXT')
      expect(w.get('[data-test="review"]').text()).toContain('brief.md (new file)')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(writeFileMock).toHaveBeenCalledWith('ws', 'brief.md', 'BRIEF TEXT')
    })

    it('still opens, blank, when the playbook cannot be read', async () => {
      getTemplateMock.mockRejectedValue(new Error('gone'))
      const w = await mountForm(null, [], { workspace: 'ws', playbook: 'brief' })
      await flushPromises()
      expect((control(w, 'Task').element as HTMLTextAreaElement).value).toBe('')
      expect((control(w, 'Workspace').element as HTMLSelectElement).value).toBe('ws')
    })

    it('ignores a playbook link when editing', async () => {
      const w = await mountForm(AUTO, [], { workspace: 'ws', playbook: 'brief' })
      await flushPromises()
      expect(listTemplatesMock).not.toHaveBeenCalled()
      expect(control(w, 'Task file').element.tagName).toBe('SELECT')
    })

    it('counts written text as unsaved changes', async () => {
      const w = await mountForm()
      await newTask(w)
      await control(w, 'Task').setValue('text')
      expect(w.text()).toContain('Unsaved changes')
    })
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

      await chooseSchedule(w, 'manual')
      expect(toggle()).toBeUndefined()
      expect(w.get('[data-test="review"]').text()).toContain('Waits its turn')
    })

    it('never sends skip_if_busy for a manual automation, even if it was switched on before', async () => {
      const w = await mountForm({ ...AUTO, skip_if_busy: true })
      await chooseSchedule(w, 'manual')
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
