import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory, type Router } from 'vue-router'
import { createAppRouter } from '../../router'
import { useConfirm } from '../../composables/ui/useConfirm'
import type { AutomationRun } from '../../types/dispatcher'

const d = {
  fetchGlobalActivity: vi.fn(),
  deleteRun: vi.fn(),
  deleteAutomationRuns: vi.fn(),
}
vi.mock('../../composables/automation/useDispatcher', () => ({ useDispatcher: () => d }))

const logs = {
  processLogLines: ref('llama-server: listening on :9001'),
  processLogRunning: ref(true),
  processLogName: ref('qwen3-8b'),
  processLogReady: ref(true),
  appLogLines: ref('INFO started'),
  appLogsFetched: ref(true),
  appLogsActive: ref(false),
  clearProcessLogs: vi.fn(),
  clearAppLogs: vi.fn(),
}
vi.mock('../../composables/system/useLogs', () => ({ useLogs: () => logs }))
const level = { logLevel: ref('INFO'), updateLogLevel: vi.fn() }
vi.mock('../../composables/system/useMetrics', () => ({ useLogLevel: () => level }))

import ActivityView from '../../views/ActivityView.vue'

const run = (id: string, over: Partial<AutomationRun> = {}): AutomationRun =>
  ({ id: `run_${id}`, workspace_id: 'ws', automation_name: `auto-${id}`, timestamp: `2026-09-29T10:0${id}:00Z`, error: '', output: '', duration_ms: 8403, model: 'qwen', ...over })
const RUNS = [run('1'), run('2', { error: 'model not configured' }), run('3', { workspace_id: 'lab' })]

const mounted: VueWrapper[] = []
async function mountActivity(path = '/activity'): Promise<{ w: VueWrapper; router: Router }> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const w = mount(ActivityView, {
    global: { plugins: [router], stubs: { Icon: true, BrandMark: true, HistoricalRunDetails: { props: ['run'], template: '<div data-test="run-details">{{ run.id }}</div>' } } },
    attachTo: document.body,
  })
  mounted.push(w)
  await flushPromises()
  return { w, router }
}
const rows = (w: VueWrapper) => w.findAll('tbody tr')

describe('Activity', () => {
  beforeEach(() => {
    d.fetchGlobalActivity.mockReset().mockResolvedValue(RUNS)
    d.deleteRun.mockReset().mockResolvedValue(undefined)
    logs.clearProcessLogs.mockReset()
    level.updateLogLevel.mockReset()
  })
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('lists every run newest first with outcome, duration and ID', async () => {
    const { w } = await mountActivity()
    expect(w.get('h1').text()).toBe('Activity')
    expect(rows(w).map((r) => r.find('td[data-label="Automation"]').text())).toEqual([
      expect.stringContaining('auto-3'),
      expect.stringContaining('auto-2'),
      expect.stringContaining('auto-1'),
    ])
    expect(rows(w)[1]!.text()).toContain('Failed')
    expect(rows(w)[1]!.text()).toContain('model not configured')
    expect(rows(w)[0]!.text()).toContain('8.403s')
    expect(rows(w)[0]!.find('button[aria-label="Copy ID run_3"]').exists()).toBe(true)
  })

  it('applies the filters from the URL and writes changes back to it', async () => {
    const { w, router } = await mountActivity('/activity?status=failed')
    expect(rows(w)).toHaveLength(1)
    await w.get('select[aria-label="Workspace"]').setValue('lab')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ status: 'failed', workspace: 'lab' })
    expect(w.text()).toContain('No runs match these filters')

    await w.findAll('button').find((b) => b.text() === 'Clear filters')!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({})
    expect(rows(w)).toHaveLength(3)
  })

  it('debounces the search into the URL', async () => {
    const { w, router } = await mountActivity()
    await w.get('input[type="search"]').setValue('lab')
    expect(router.currentRoute.value.query).toEqual({})
    await vi.waitFor(() => expect(router.currentRoute.value.query).toEqual({ q: 'lab' }))
    expect(rows(w)).toHaveLength(1)
  })

  it('opens a run in the drawer, with the run in the URL until it closes', async () => {
    const { w, router } = await mountActivity('/activity?status=failed')
    await rows(w)[0]!.trigger('click')
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('run_2')
    expect(router.currentRoute.value.query).toEqual({ status: 'failed', run: 'run_2' })
    ;(document.body.querySelector('button[aria-label="Close Run details"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
    expect(router.currentRoute.value.query).toEqual({ status: 'failed' })
  })

  it('opens the run a link names', async () => {
    await mountActivity('/activity?run=run_3')
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('run_3')
  })

  it('says when a linked run is no longer in the history', async () => {
    const { w } = await mountActivity('/activity?run=run_gone')
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('no longer in the run history')
    expect(rows(w)).toHaveLength(3)
  })

  it('fetches again for a linked run newer than the list it holds', async () => {
    const { router } = await mountActivity()
    d.fetchGlobalActivity.mockResolvedValue([...RUNS, run('4')])
    await router.push('/activity?run=run_4')
    expect(document.body.querySelector('[role="dialog"]')?.textContent).not.toContain('no longer in the run history')
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('run_4')
  })

  it('follows a row\'s automation link without opening the run', async () => {
    const { w, router } = await mountActivity()
    await rows(w)[2]!.get('td[data-label="Automation"] a').trigger('click')
    // The automation route is a lazy chunk: navigation settles after its import.
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('automation'))
    expect(router.currentRoute.value.query).toEqual({})
  })

  it('links each run to its automation and its workspace', async () => {
    const { w } = await mountActivity()
    const row = rows(w)[2]!
    expect(decodeURIComponent(row.get('td[data-label="Automation"] a').attributes('href')!)).toBe('/automations/ws/auto-1')
    expect(row.get('td[data-label="Workspace"] a').attributes('href')).toBe('/workspaces/ws')
  })

  it('reports a failed load with a retry, and an empty ledger', async () => {
    d.fetchGlobalActivity.mockImplementation(async () => {
      throw new Error('timed out after 10s')
    })
    const failed = await mountActivity()
    expect(failed.w.get('[role="alert"]').text()).toContain('timed out after 10s')

    d.fetchGlobalActivity.mockReset().mockResolvedValue([])
    const empty = await mountActivity()
    expect(empty.w.text()).toContain('No runs yet')
  })

  it('shows the process log with its status, and clears it only after confirming', async () => {
    const { w } = await mountActivity()
    await w.findAll('[role="radio"]').find((r) => r.text() === 'Process log')!.trigger('click')
    expect(w.get('[role="region"][aria-label="Process log lines"]').text()).toContain('listening on :9001')
    expect(w.text()).toContain('qwen3-8b')

    await w.findAll('button').find((b) => b.text() === 'Clear log')!.trigger('click')
    await flushPromises()
    expect(logs.clearProcessLogs).not.toHaveBeenCalled()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(logs.clearProcessLogs).toHaveBeenCalledTimes(1)
  })

  it('reads the app log only while it is shown, and changes the log level', async () => {
    const { w } = await mountActivity()
    expect(logs.appLogsActive.value).toBe(false)
    await w.findAll('[role="radio"]').find((r) => r.text() === 'App log')!.trigger('click')
    expect(logs.appLogsActive.value).toBe(true)
    await w.findAll('[role="radio"]').find((r) => r.text() === 'DEBUG')!.trigger('click')
    expect(level.updateLogLevel).toHaveBeenCalledWith('DEBUG')
  })
})
