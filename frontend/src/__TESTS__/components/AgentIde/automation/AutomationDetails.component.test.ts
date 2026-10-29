import { describe, it, expect, vi, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../../router'
import { useConfirm } from '../../../../composables/ui/useConfirm'
import type { Automation, AutomationRun } from '../../../../types/dispatcher'

const profileService = vi.hoisted(() => ({
  list: vi.fn().mockResolvedValue([
    { id: 'research', name: 'Research', summary: 'Finds and cites.', source: 'builtin', answer_first: true, may_ask: true, memory_save: 'model', reminders: false, default_for: [] },
    { id: 'task-runner', name: 'Task runner', summary: 'Runs unattended.', source: 'builtin', answer_first: false, may_ask: false, memory_save: 'model', reminders: false, default_for: ['automation'] },
  ]),
}))
vi.mock('../../../../services/profile/profileService', () => ({ ProfileService: profileService }))

const connect = vi.fn()
vi.mock('../../../../composables/automation/useLiveConsole', () => ({
  useLiveConsole: () => ({
    messages: ref([]), thinking: ref(false), liveReasoning: ref(''), paused: ref(false), phase: ref('idle'),
    isConnected: ref(true), pendingDecision: ref(null), connect, disconnect: vi.fn(), clearEvents: vi.fn(), submitDecision: vi.fn(),
  }),
}))

const { getJournal, clearJournal, getReports } = vi.hoisted(() => ({
  getJournal: vi.fn(),
  clearJournal: vi.fn(),
  getReports: vi.fn().mockResolvedValue({ reports: [], truncated: false }),
}))
vi.mock('../../../../services/automation/dispatcherService', () => ({
  DispatcherService: { getAutomationJournal: getJournal, clearAutomationJournal: clearJournal, getAutomationReports: getReports, readWorkspaceFile: vi.fn() },
}))

const netaudit = vi.hoisted(() => ({
  findings: vi.fn().mockResolvedValue({ scanned: false, findings: [], resolved: [], unscanned: [], devices_up: 0 }),
  readiness: vi.fn().mockResolvedValue({ ready: true, requirements: [] }),
}))
vi.mock('../../../../services/netaudit/netauditService', () => ({ NetauditService: netaudit }))

import AutomationDetails from '../../../../components/AgentIde/automation/AutomationDetails.vue'

const run = (id: string, over: Partial<AutomationRun> = {}): AutomationRun =>
  ({ id: `run_${id}`, workspace_id: 'ws', automation_name: 'nightly', timestamp: `2026-09-29T10:0${id}:00Z`, error: '', output: `out ${id}`, duration_ms: 8403, model: '', ...over })
const AUTO: Automation = {
  id: 'ws/nightly', workspace: 'ws', name: 'nightly', task_file: 'jobs/nightly.md', strategy: 'persistent', trigger: 'cron', trigger_value: '0 7 * * *',
  model: 'qwen', last_output: 'All good', last_error: 'model timed out', history: [run('1'), run('2', { error: 'boom' })],
}
const STUBS = {
  Icon: true,
  BrandMark: true,
  ChatMessages: { template: '<div data-test="console" />' },
  GuardrailBanner: true,
  MarkdownViewer: { props: ['content'], template: '<div class="md">{{ content }}</div>' },
}

const mounted: VueWrapper[] = []
async function mountDetails(props: Record<string, unknown> = {}) {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/automations/ws%2Fnightly')
  const w = mount(AutomationDetails, { props: { automation: AUTO, ...props }, global: { plugins: [router], stubs: STUBS }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return w
}

// Characterised against the old detail view first (plan D22), then updated to
// the detail page: same content and actions, as panels and a runs table.
describe('AutomationDetails', () => {
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('shows the network scan report on an auditor automation only', async () => {
    const plain = await mountDetails()
    expect(plain.text()).not.toContain('Latest network scan')
    expect(netaudit.findings).not.toHaveBeenCalled()
    const w = await mountDetails({ automation: { ...AUTO, profile: 'security-auditor' } })
    expect(w.text()).toContain('Latest network scan')
    expect(w.text()).toContain('Not scanned yet')
    expect(netaudit.findings).toHaveBeenCalledWith('ws', 'nightly')
    expect(w.find('button[aria-checked="true"]').text()).toBe('Simple')
  })

  it('reads the scan report again when the run finishes, without a page refresh', async () => {
    netaudit.findings.mockClear()
    const audit = { ...AUTO, profile: 'security-auditor' }
    const w = await mountDetails({ automation: audit, isExecuting: true })
    expect(netaudit.findings).toHaveBeenCalledTimes(1)
    await w.setProps({ isExecuting: false, automation: { ...audit, history: [run('3'), ...(audit.history ?? [])] } })
    await flushPromises()
    expect(netaudit.findings.mock.calls.length).toBeGreaterThanOrEqual(2) // it was read again after the run, not only on mount
  })

  it('says in one plain line what an audit scans, and nothing for an automation that is not one', async () => {
    const audit = await mountDetails({ automation: { ...AUTO, profile: 'security-auditor', scan: { depth: 'quick', checks: false, targets: ['192.168.1.0/24'] } } })
    expect(audit.get('[data-test="scans"]').findAll('[data-test="scan-tag"]').map((t) => t.text())).toEqual(['Quick scan', 'Checks off', '192.168.1.0/24'])
    const plain = await mountDetails()
    expect(plain.text()).not.toContain('Scans')
  })

  it('lists the automation\'s saved reports, loaded for that automation', async () => {
    getReports.mockResolvedValueOnce({ reports: [{ path: 'reports/nightly/20261021T070000Z-run_2.md', run_id: 'run_2', started_at: '2026-10-21T07:00:00Z', size_bytes: 512 }], truncated: false })
    const w = await mountDetails()
    expect(getReports).toHaveBeenCalledWith('ws', 'nightly')
    expect(w.text()).toContain('Saved reports')
    expect(w.text()).toContain('run_2')
  })

  it('links the last result to its saved report file', async () => {
    const w = await mountDetails({ automation: { ...AUTO, last_report_path: 'reports/nightly/20261021T070000Z-run_1.md' } })
    const link = w.get('a[aria-label="Open saved report"]')
    expect(decodeURIComponent(link.attributes('href')!)).toBe('/workspaces/ws/files/reports/nightly/20261021T070000Z-run_1.md')
  })

  it('shows no saved-report link when the last run kept none', async () => {
    const w = await mountDetails()
    expect(w.find('a[aria-label="Open saved report"]').exists()).toBe(false)
  })

  it('says what memory the runs get: an override as itself, an inherited value as the default', async () => {
    const memoryLine = async (memory_mode: Automation['memory_mode']) => {
      const w = await mountDetails({ automation: { ...AUTO, memory_mode } })
      return w.text()
    }
    expect(await memoryLine('on')).toMatch(/Memory\s*On/)
    expect(await memoryLine('off')).toMatch(/Memory\s*Off/)
    expect(await memoryLine('')).toMatch(/Memory\s*Default \(off\)/)
  })

  it('names the model a run uses in the console, the automation\'s own or else the primary', async () => {
    const own = await mountDetails()
    expect(own.get('[data-test="console-model"]').text()).toBe('qwen')

    const { useModels } = await import('../../../../composables/models/useModels')
    useModels().state.value = { models: [], available: [], next_port: 9000, config: { primary_model: 'ornith' } } as never
    const inherited = await mountDetails({ automation: { ...AUTO, model: '' } })
    expect(inherited.get('[data-test="console-model"]').text()).toBe('ornith')
    expect(inherited.text()).toContain('Default (ornith)')
    useModels().state.value = null
  })

  it("names the profile its runs use, or the default's name when none was chosen", async () => {
    const chosen = await mountDetails({ automation: { ...AUTO, profile: 'research' } })
    await flushPromises()
    expect(chosen.text()).toMatch(/Agent\s*Research/)
    const unchosen = await mountDetails()
    await flushPromises()
    expect(unchosen.text()).toMatch(/Agent\s*Default \(Task runner\)/)
  })

  it('shows the configuration, the last error and the last output, and connects the live console', async () => {
    const w = await mountDetails()
    for (const text of ['Every day at 07:00', 'persistent', 'qwen', 'jobs/nightly.md', 'model timed out', 'All good']) {
      expect(w.text()).toContain(text)
    }
    expect(w.get('a[href="/workspaces/ws/files/jobs/nightly.md"]').text()).toBe('jobs/nightly.md')
    expect(connect).toHaveBeenCalled()
  })

  it('shows the next run with its clock, and nothing for a manual automation', async () => {
    const scheduled = await mountDetails({ automation: { ...AUTO, next_run_at: '2026-10-12T07:00:00+01:00' } })
    expect(scheduled.text()).toMatch(/Next run\s*Mon 12 Oct, 07:00 \(server time\)/)
    const manual = await mountDetails({ automation: { ...AUTO, trigger: 'manual', trigger_value: '', next_run_at: undefined } })
    expect(manual.text()).not.toContain('Next run')
  })

  it('shows where results are delivered and what a busy model does', async () => {
    const quiet = await mountDetails()
    expect(quiet.text()).toContain('Not sent')
    expect(quiet.text()).toContain('Waits its turn')

    const w = await mountDetails({ automation: { ...AUTO, notify: { connector: 'my-telegram', dedup: true }, skip_if_busy: true } })
    expect(w.text()).toContain('my-telegram · skips repeats')
    expect(w.text()).toContain('Skips the run')
  })

  it('copies the last error and the last output', async () => {
    const copies = (await mountDetails()).findAllComponents({ name: 'CopyButton' })
    expect(copies.map((c) => [c.props('title'), c.props('text')])).toEqual([
      ['Copy last error', 'model timed out'],
      ['Copy last output', 'All good'],
    ])
  })

  it('says the console is idle instead of showing an empty stream', async () => {
    const w = await mountDetails()
    expect(w.find('[data-test="console"]').exists()).toBe(false)
    expect(w.text()).toContain('Idle')
    expect(w.text()).not.toContain('Live stream')
    expect(w.text()).not.toContain('Not running')
  })

  it('says why a finished run has no steps here, instead of "not running"', async () => {
    const w = await mountDetails()
    expect(w.text()).toContain('No steps to show')
    expect(w.text()).toContain('memory only')
  })

  it('says a run\'s steps stream here once the automation has never run', async () => {
    const w = await mountDetails({ automation: { ...AUTO, last_output: '', last_error: '', history: [] } })
    expect(w.text()).toContain('Not running')
  })

  it('lists past runs newest first, opens one, and deletes one or all only after confirming', async () => {
    const w = await mountDetails()
    const rows = w.findAll('tbody tr')
    expect(rows.map((r) => r.text())).toEqual([expect.stringContaining('Failed'), expect.stringContaining('Completed')])
    await rows[0]!.trigger('click')
    expect(w.emitted('open-run')).toEqual([[AUTO.history![1]]])

    await w.get('button[aria-label="Delete run run_2"]').trigger('click')
    await flushPromises()
    expect(w.emitted('delete-run')).toBeUndefined()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete-run')).toEqual([[AUTO.history![1]]])

    await w.findAll('button').find((b) => b.text().includes('Clear all runs'))!.trigger('click')
    await flushPromises()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(w.emitted('delete-automation-runs')).toEqual([[AUTO]])
  })

  it('shows the live state while running', async () => {
    const w = await mountDetails({ isExecuting: true, lastTriggerResult: 'Running nightly...' })
    expect(w.text()).toContain('Running nightly...')
    expect(w.find('[data-test="console"]').exists()).toBe(true)
    expect(w.text()).toContain('Live stream')
    expect(w.text()).not.toContain('All good')
  })

  it('says a run completed with warnings and shows why, in the last result and in the runs table', async () => {
    const warning = 'report not delivered via tg: telegram API error: status 401'
    const w = await mountDetails({
      automation: { ...AUTO, last_error: '', last_warnings: [warning], history: [run('1', { warnings: [warning] })] },
    })
    expect(w.text()).toContain('Completed with warnings')
    expect(w.text()).toContain(warning)
    expect(w.findAll('table tbody tr').some((r) => r.text().includes('Completed with warnings'))).toBe(true)
  })

  it('says when it has never run', async () => {
    const w = await mountDetails({ automation: { ...AUTO, last_output: '', last_error: '', history: [] } })
    expect(w.text()).toContain('No runs yet')
  })

  describe('learning journal', () => {
    const KEEPS = { ...AUTO, journal: true }
    const headings = (w: VueWrapper) => w.findAll('h2').map((h) => h.text())

    it('shows no Journal panel and never fetches while the automation keeps none', async () => {
      getJournal.mockClear()
      const w = await mountDetails()
      expect(headings(w)).not.toContain('Journal')
      expect(w.text()).toContain('Off')
      expect(getJournal).not.toHaveBeenCalled()
    })

    it('shows the notes the automation left for itself', async () => {
      getJournal.mockResolvedValue({ journal: '- query: llm release notes' })
      const w = await mountDetails({ automation: KEEPS })
      expect(headings(w)).toContain('Journal')
      expect(w.text()).toContain('Keeps notes between runs')
      expect(w.findAll('.md').map((m) => m.text())).toContain('- query: llm release notes')
    })

    it('says so when nothing has been written yet', async () => {
      getJournal.mockResolvedValue({ journal: '' })
      const w = await mountDetails({ automation: KEEPS })
      expect(w.text()).toContain('No notes yet')
      expect(w.findAll('button').some((b) => b.text().includes('Clear journal'))).toBe(false)
    })

    it('clears the journal only after confirming', async () => {
      getJournal.mockResolvedValue({ journal: '- query A' })
      clearJournal.mockResolvedValue(undefined)
      const w = await mountDetails({ automation: KEEPS })
      await w.findAll('button').find((b) => b.text().includes('Clear journal'))!.trigger('click')
      await flushPromises()
      expect(clearJournal).not.toHaveBeenCalled()
      useConfirm().handleConfirm()
      await flushPromises()
      expect(clearJournal).toHaveBeenCalledWith('ws', 'nightly')
      expect(w.text()).toContain('No notes yet')
    })

    it('explains a journal that could not be loaded', async () => {
      getJournal.mockRejectedValue(new Error('boom'))
      const w = await mountDetails({ automation: KEEPS })
      expect(w.get('[role="alert"]').text()).toContain('Could not load the journal')
    })
  })
})

// An audit's page is split into tabs so that one screen holds one job: reading results, watching the run, the history,
// and the setup. Every other automation keeps the single page.
describe('AutomationDetails of a network audit', () => {
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })
  const OBSERVED = '## Observed (generated from the scan data, not written by the model)\n\nScanned 254 addresses.'
  const ASSESSMENT = '## Assessment (written by the model)\n\nSamba is old.'
  const AUDIT: Automation = { ...AUTO, profile: 'security-auditor', last_error: '', last_output: `${OBSERVED}\n\n---\n\n${ASSESSMENT}`, scan: { depth: 'quick', checks: false, targets: ['192.168.1.0/24'] } }
  const tab = (w: VueWrapper, label: string) => w.findAll('[role="radio"]').find((b) => b.text().startsWith(label))!

  it('opens on the results, with the configuration, runs and console on other tabs', async () => {
    const w = await mountDetails({ automation: AUDIT })
    expect(tab(w, 'Results').attributes('aria-checked')).toBe('true')
    expect(w.text()).toContain('Latest network scan')
    for (const other of ['Task file', 'Saved reports', 'Last result', 'Runs of this automation']) expect(w.text()).not.toContain(other)
    expect(w.find('[data-test="console"]').exists()).toBe(false)
  })

  it('says what the audit scans on the results tab, and keeps the rest of the configuration for Setup', async () => {
    const w = await mountDetails({ automation: AUDIT })
    expect(w.get('[data-test="scans"]').findAll('[data-test="scan-tag"]').map((t) => t.text())).toEqual(['Quick scan', 'Checks off', '192.168.1.0/24'])
    await tab(w, 'Setup').trigger('click')
    expect(w.text()).toContain('Task file')
    expect(w.text()).not.toContain('Latest network scan')
  })

  it('shows the console on its own tab and the history on another', async () => {
    const w = await mountDetails({ automation: AUDIT })
    await tab(w, 'Live run').trigger('click')
    expect(w.find('[data-test="console"], [data-test="console-model"]').exists()).toBe(true)
    await tab(w, 'History').trigger('click')
    expect(w.text()).toContain('Saved reports')
    expect(w.text()).toContain('Runs')
  })

  it('shows only the model\'s assessment as the last result, since the scan data is the Results tab', async () => {
    const w = await mountDetails({ automation: AUDIT })
    await tab(w, 'History').trigger('click')
    const result = w.get('.md')
    expect(result.text()).toContain('Samba is old.')
    expect(result.text()).not.toContain('Scanned 254 addresses.')
  })

  it('turns the report off while it runs, and the banner takes you to the live run', async () => {
    netaudit.findings.mockResolvedValueOnce({ scanned: true, scanned_at: '2026-10-26T02:00:00Z', engine: 'nmap', devices_up: 1, resolved: [], unscanned: [], findings: [{ key: 'p', device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false }] })
    const w = await mountDetails({ automation: AUDIT, isExecuting: true })
    expect(w.get('[data-test="report-controls"]').attributes('disabled')).toBeDefined()
    await w.get('[data-test="watch-live"]').trigger('click')
    expect(tab(w, 'Live run').attributes('aria-checked')).toBe('true')
  })

  it('shows the live run tab with a dot and tints the chosen tab while a run is on, and neither when idle', async () => {
    const busy = await mountDetails({ automation: AUDIT, isExecuting: true })
    expect(tab(busy, 'Live run').find('[data-test="live-dot"]').exists()).toBe(true)
    expect(tab(busy, 'Results').classes().join(' ')).toContain('state-running')
    const idle = await mountDetails({ automation: AUDIT })
    expect(idle.find('[data-test="live-dot"]').exists()).toBe(false)
    expect(tab(idle, 'Results').classes().join(' ')).not.toContain('state-running')
  })

  it('says on the results tab that the last run failed, and leads to why', async () => {
    const w = await mountDetails({ automation: { ...AUDIT, last_error: 'nmap is not installed' } })
    const note = w.get('[data-test="last-run-failed"]')
    expect(note.text()).toContain('The last run failed')
    await note.get('button').trigger('click')
    expect(tab(w, 'History').attributes('aria-checked')).toBe('true')
    expect(w.text()).toContain('nmap is not installed')
  })

  it('says nothing about a failure when the last run worked', async () => {
    const w = await mountDetails({ automation: AUDIT })
    expect(w.find('[data-test="last-run-failed"]').exists()).toBe(false)
  })

  it('turns off clearing runs and the journal while it runs', async () => {
    const w = await mountDetails({ automation: { ...AUDIT, journal: true }, isExecuting: true })
    await tab(w, 'History').trigger('click')
    for (const name of ['Clear all runs', 'Clear journal']) {
      const b = w.findAll('button').find((x) => x.text().includes(name))
      if (b) expect(b.attributes('disabled')).toBeDefined()
    }
    expect(w.findAll('button').find((x) => x.text().includes('Clear all runs'))!.attributes('disabled')).toBeDefined()
  })
})
