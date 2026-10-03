import { describe, it, expect, vi, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../../router'
import { useConfirm } from '../../../../composables/ui/useConfirm'
import type { Automation, AutomationRun } from '../../../../types/dispatcher'

const connect = vi.fn()
vi.mock('../../../../composables/automation/useLiveConsole', () => ({
  useLiveConsole: () => ({
    messages: ref([]), thinking: ref(false), liveReasoning: ref(''), paused: ref(false), phase: ref('idle'),
    isConnected: ref(true), pendingDecision: ref(null), connect, disconnect: vi.fn(), clearEvents: vi.fn(), submitDecision: vi.fn(),
  }),
}))

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

  it('shows the configuration, the last error and the last output, and connects the live console', async () => {
    const w = await mountDetails()
    for (const text of ['At 07:00 AM', 'persistent', 'qwen', 'jobs/nightly.md', 'model timed out', 'All good']) {
      expect(w.text()).toContain(text)
    }
    expect(w.get('a[href="/workspaces/ws/files/jobs/nightly.md"]').text()).toBe('jobs/nightly.md')
    expect(connect).toHaveBeenCalled()
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

  it('says when it has never run', async () => {
    const w = await mountDetails({ automation: { ...AUTO, last_output: '', last_error: '', history: [] } })
    expect(w.text()).toContain('No runs yet')
  })
})
