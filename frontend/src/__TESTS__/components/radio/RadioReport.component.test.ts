import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { routes } from '../../../router'

const dispatcher = vi.hoisted(() => ({ listAutomations: vi.fn(), listWorkspaces: vi.fn(), triggerAutomation: vi.fn(), createAutomation: vi.fn(), readWorkspaceFile: vi.fn() }))
const templates = vi.hoisted(() => ({ getTemplate: vi.fn() }))
const writer = vi.hoisted(() => ({ writeFile: vi.fn() }))
const radio = vi.hoisted(() => ({ sensors: vi.fn(), listening: vi.fn() }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: dispatcher }))
vi.mock('../../../services/template/templateService', () => ({ TemplateService: templates }))
vi.mock('../../../composables/editor/useWorkspaceFileWriter', () => ({ useWorkspaceFileWriter: () => writer }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: radio }))

import RadioReport from '../../../components/radio/RadioReport.vue'

const idle = { listening: false, cycles: 0, frames: 0, windows: [], plan: { channels: [1], window_seconds: 20, cycle_seconds: 300 } }
// The server's last_output is the run's log (executor lines around the report); the clean report is the saved file at last_report_path.
const LOG = '[2026-11-16 20:07:57] ▶ Executing automation `radio-watch-report` in workspace `workspace-1`...\nReading task file: `radio-watch-report.md`\n\n⏱ **Duration:** 32s\n'
const existing = { id: 'ws1/radio-watch-report', workspace: 'ws1', name: 'radio-watch-report', task_file: 'radio-watch-report.md', strategy: '', trigger: 'manual', trigger_type: 'manual' }

beforeEach(() => {
  vi.clearAllMocks()
  dispatcher.listAutomations.mockResolvedValue([])
  dispatcher.listWorkspaces.mockResolvedValue([{ id: 'ws1' }])
  dispatcher.createAutomation.mockResolvedValue(undefined)
  dispatcher.triggerAutomation.mockResolvedValue({})
  dispatcher.readWorkspaceFile.mockResolvedValue('')
  templates.getTemplate.mockResolvedValue({ id: 'radio-watch-report', content: 'TASK', defaults: { profile: 'radio-reporter', schedule: '0 7 * * *', journal: false } })
  writer.writeFile.mockResolvedValue('created')
  radio.sensors.mockResolvedValue({ enabled: true, unused: 0, sensors: [] })
  radio.listening.mockResolvedValue({ ...idle, listening: true })
})

afterEach(() => vi.useRealTimers())

async function show() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  const w = mount(RadioReport, { global: { plugins: [router] } })
  await flushPromises()
  return w
}

describe('RadioReport', () => {
  it('sets the report up as a manual automation by default, then runs it once', async () => {
    const w = await show()
    await w.get('[data-test="radio-report-go"]').trigger('click')
    await flushPromises()
    expect(writer.writeFile).toHaveBeenCalledWith('ws1', 'radio-watch-report.md', 'TASK')
    expect(dispatcher.createAutomation).toHaveBeenCalledWith('ws1', expect.objectContaining({
      name: 'radio-watch-report', trigger: { type: 'manual', value: '' }, task_file: 'radio-watch-report.md', profile: 'radio-reporter', skip_if_busy: false, notify: null,
    }))
    expect(dispatcher.triggerAutomation).toHaveBeenCalledWith('ws1', 'radio-watch-report')
  })

  it('can set it up on the playbook’s own morning schedule and still runs it once now', async () => {
    const w = await show()
    await w.findAll('button').find((b) => b.text() === 'Every morning at 7:00')!.trigger('click')
    await w.get('[data-test="radio-report-go"]').trigger('click')
    await flushPromises()
    expect(dispatcher.createAutomation.mock.calls[0]![1]).toMatchObject({ trigger: { type: 'cron', value: '0 7 * * *' }, skip_if_busy: true })
    expect(dispatcher.triggerAutomation).toHaveBeenCalledTimes(1)
  })

  it('only runs an automation that exists, and creates nothing', async () => {
    dispatcher.listAutomations.mockResolvedValue([{ ...existing, last_output: LOG + 'No findings.', last_report_path: 'reports/radio-watch-report/a/report.md' }])
    dispatcher.readWorkspaceFile.mockResolvedValue('No findings.')
    const w = await show()
    expect(w.find('[data-test="radio-report-setup"]').exists()).toBe(false)
    expect(w.get('[data-test="radio-report-runs"]').text()).toBe('Runs only when you press Analyse.')
    expect(dispatcher.readWorkspaceFile).toHaveBeenCalledWith('ws1', 'reports/radio-watch-report/a/report.md')
    expect(w.get('[data-test="radio-report-output"]').text()).toBe('No findings.')
    expect(w.text()).not.toContain('Executing automation')
    await w.get('[data-test="radio-report-go"]').trigger('click')
    await flushPromises()
    expect(dispatcher.createAutomation).not.toHaveBeenCalled()
    expect(dispatcher.triggerAutomation).toHaveBeenCalledWith('ws1', 'radio-watch-report')
  })

  it('says it cannot start listening, with the way to start it, and still lets the report run', async () => {
    radio.listening.mockResolvedValue(idle)
    const w = await show()
    expect(w.get('[data-test="radio-report-notlistening"]').text()).toContain('Not listening now')
    expect(w.find('[data-test="radio-report-start"]').exists()).toBe(true)
    expect(w.get('[data-test="radio-report-go"]').attributes('disabled')).toBeUndefined()
  })

  it('offers nothing to run while the Radio stack is off, and says how to fix it', async () => {
    radio.sensors.mockResolvedValue({ enabled: false, unused: 0, sensors: [] })
    const w = await show()
    expect(w.get('[data-test="radio-report-off"]').text()).toContain('Switch the Radio stack on')
    expect(w.get('[data-test="radio-report-go"]').attributes('disabled')).toBeDefined()
  })

  it('shows a run that is going and a run that failed, in plain words', async () => {
    dispatcher.listAutomations.mockResolvedValue([{ ...existing, is_running: true }])
    const running = await show()
    expect(running.find('[data-test="radio-report-running"]').exists()).toBe(true)
    expect(running.get('[data-test="radio-report-go"]').attributes('disabled')).toBeDefined()
    dispatcher.listAutomations.mockResolvedValue([{ ...existing, last_error: 'this run may only use a local model' }])
    const failed = await show()
    expect(failed.get('[data-test="radio-report-last-error"]').text()).toContain('local model')
  })

  it('creates nothing when replacing an existing task file is declined', async () => {
    writer.writeFile.mockResolvedValue('cancelled')
    const w = await show()
    await w.get('[data-test="radio-report-go"]').trigger('click')
    await flushPromises()
    expect(dispatcher.createAutomation).not.toHaveBeenCalled()
    expect(dispatcher.triggerAutomation).not.toHaveBeenCalled()
  })

  it('says when there is no workspace to put it in', async () => {
    dispatcher.listWorkspaces.mockResolvedValue([])
    const w = await show()
    await w.get('[data-test="radio-report-go"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="radio-report-error"]').text()).toContain('no workspace')
  })

  it('keeps waiting when the first look after a press still shows the old result, and shows the new one when it lands', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    const old = { ...existing, last_output: LOG, last_report_path: 'reports/radio-watch-report/old/report.md' }
    dispatcher.readWorkspaceFile.mockImplementation(async (_ws: string, path: string) => (path.includes('/new/') ? 'New report.' : 'Old report.'))
    dispatcher.listAutomations.mockResolvedValueOnce([old]) // the card's own first look
    const w = await show()
    expect(w.get('[data-test="radio-report-output"]').text()).toBe('Old report.')
    dispatcher.listAutomations.mockResolvedValueOnce([old]) // right after the press: the run has not shown yet
    await w.get('[data-test="radio-report-go"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-test="radio-report-running"]').exists()).toBe(true)
    expect(w.find('[data-test="radio-report-output"]').exists()).toBe(false)
    dispatcher.listAutomations.mockResolvedValueOnce([{ ...old, is_running: true }])
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(w.find('[data-test="radio-report-running"]').exists()).toBe(true)
    dispatcher.listAutomations.mockResolvedValueOnce([{ ...old, last_report_path: 'reports/radio-watch-report/new/report.md' }])
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(w.find('[data-test="radio-report-running"]').exists()).toBe(false)
    expect(w.get('[data-test="radio-report-output"]').text()).toBe('New report.')
  })

  it('tells a schedule in words, never as a cron string', async () => {
    dispatcher.listAutomations.mockResolvedValue([{ ...existing, trigger: 'cron', trigger_type: 'cron', trigger_value: '0 7 * * *', next_run_at: '2026-11-17T07:00:00Z' }])
    const w = await show()
    const line = w.get('[data-test="radio-report-runs"]').text()
    expect(line).toContain('Runs every morning at 7:00; next run')
    expect(line).not.toContain('* *')
    dispatcher.listAutomations.mockResolvedValue([{ ...existing, trigger: 'cron', trigger_type: 'cron', trigger_value: '30 2 * * 1' }])
    expect((await show()).get('[data-test="radio-report-runs"]').text()).toBe('Runs on its own schedule.')
  })

  it('shows the saved report as formatted text, not as markdown symbols', async () => {
    dispatcher.listAutomations.mockResolvedValue([{ ...existing, last_output: LOG, last_report_path: 'reports/radio-watch-report/a/report.md' }])
    dispatcher.readWorkspaceFile.mockResolvedValue('**Listening:** 94 cycles.\n\n- first finding\n- second finding')
    const w = await show()
    const out = w.get('[data-test="radio-report-output"]')
    expect(out.findAll('li')).toHaveLength(2)
    expect(out.text()).not.toContain('**')
    expect(out.html()).toContain('<strong>')
  })
})

