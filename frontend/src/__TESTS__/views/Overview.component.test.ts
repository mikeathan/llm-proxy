import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref, shallowRef } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../router'
import type { ActiveModel } from '../../types/model'
import type { LaneHolder, QueuedRun, LaneSummary } from '../../types/assistant'
import type { MetricsSample, SystemMetrics } from '../../types/metrics'
import type { AutomationRun } from '../../types/dispatcher'

const state = {
  activeModel: ref<ActiveModel | null>(null),
  stopModel: vi.fn(),
  metrics: ref<SystemMetrics | null>(null),
  history: shallowRef<MetricsSample[]>([]),
  laneHolders: ref<LaneHolder[]>([]),
  queuedRuns: ref<QueuedRun[]>([]),
  laneError: ref<string | null>(null),
  lanes: ref<LaneSummary[]>([]),
  fetchGlobalActivity: vi.fn(),
}

vi.mock('../../composables/models/useModels', () => ({
  useModels: () => ({ activeModel: state.activeModel, stopModel: state.stopModel }),
}))
vi.mock('../../composables/system/useMetrics', () => ({
  useMetrics: () => ({ metrics: state.metrics, history: state.history }),
}))
vi.mock('../../composables/assistant/useGlobalRunActivity', () => ({
  useGlobalRunActivity: () => ({ laneHolders: state.laneHolders, queuedRuns: state.queuedRuns, lanes: state.lanes, error: state.laneError }),
}))
vi.mock('../../composables/automation/useDispatcher', () => ({
  useDispatcher: () => ({ fetchGlobalActivity: state.fetchGlobalActivity }),
}))

import OverviewView from '../../views/OverviewView.vue'

const METRICS: SystemMetrics = {
  load_percent: 42,
  mem_used_mb: 8192,
  mem_total_mb: 16384,
  llm_tokens_per_sec: 48.62,
  gpu: { name: 'Apple GPU', memory_used_mb: 6144, memory_total_mb: 8192, memory_utilization_percent: 75, utilization_percent: 30, temperature_c: 61 },
}
const MODEL: ActiveModel = { name: 'qwen-local', provider: 'local', endpoint: 'http://127.0.0.1:8080', port: 8080, ready: true, started_at: '', last_used_at: '' }
const RUN = (name: string, error = ''): AutomationRun =>
  ({ id: `r-${name}`, workspace_id: 'ws', automation_name: name, timestamp: '2026-09-29T10:00:00Z', error, output: '', duration_ms: 8403, model: 'm' })

async function mountOverview() {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/overview')
  const w = mount(OverviewView, { global: { plugins: [router], stubs: { Icon: true, BrandMark: true } } })
  await flushPromises()
  return w
}

describe('Overview', () => {
  beforeEach(() => {
    state.activeModel.value = MODEL
    state.metrics.value = METRICS
    state.history.value = [{ tokensPerSecond: 40, loadPercent: 30 }, { tokensPerSecond: 48.6, loadPercent: 42 }]
    state.laneHolders.value = []
    state.queuedRuns.value = []
    state.laneError.value = null
    state.lanes.value = []
    state.stopModel.mockReset()
    state.fetchGlobalActivity.mockReset().mockResolvedValue([RUN('nightly'), RUN('broken', 'model not configured')])
  })

  it('shows health as stat tiles with history and meters', async () => {
    const w = await mountOverview()
    expect(w.get('h1').text()).toBe('Overview')
    const values = w.findAll('[data-test="stat-value"]').map((v) => v.text())
    expect(values).toEqual(['48.6', '42', '8.0', '61'])
    expect(w.findAll('[role="img"]')).toHaveLength(2)
    const meters = w.findAll('[role="meter"]').map((m) => [m.attributes('aria-label'), m.attributes('aria-valuenow')])
    expect(meters).toEqual([['Memory used', '50'], ['VRAM used', '75']])
    expect(w.text()).toContain('since page load')
  })

  it('offers stopping the active model, and loading one when none is active', async () => {
    const w = await mountOverview()
    expect(w.text()).toContain('qwen-local')
    await w.findAll('button').find((b) => b.text() === 'Stop model')!.trigger('click')
    expect(state.stopModel).toHaveBeenCalledTimes(1)

    state.activeModel.value = null
    await flushPromises()
    expect(w.text()).toContain('No model loaded')
    expect(w.find('a[href="/models"]').exists()).toBe(true)
  })

  it('says when there is no GPU', async () => {
    state.metrics.value = { ...METRICS, gpu: undefined }
    const w = await mountOverview()
    expect(w.text()).toContain('No GPU detected')
    expect(w.findAll('[role="meter"]')).toHaveLength(1)
  })

  it('shows a dash, not NaN, when the GPU reports no temperature', async () => {
    state.metrics.value = { ...METRICS, gpu: { ...METRICS.gpu!, temperature_c: undefined as unknown as number } }
    const w = await mountOverview()
    expect(w.findAll('[data-test="stat-value"]')[3]!.text()).toBe('—')
    expect(w.text()).toContain('Temperature not reported')
    expect(w.text()).not.toContain('NaN')
  })

  it('shows loading health until the first metrics arrive', async () => {
    state.metrics.value = null
    const w = await mountOverview()
    expect(w.text()).toContain('Loading health')
  })

  it('lists what is running and queued, or says nothing is', async () => {
    const idle = await mountOverview()
    expect(idle.text()).toContain('Nothing is running')

    state.laneHolders.value = [{ key: 'ws/nightly', kind: 'automation', workspace_id: 'ws', automation: 'nightly', label: 'ws/nightly', since: new Date(Date.now() - 65_000).toISOString() }]
    state.queuedRuns.value = [{ key: 'ws/b', kind: 'automation', workspace_id: 'ws', automation: 'b', label: 'ws/b', position: 1, queued_at: '' }]
    const busy = await mountOverview()
    expect(busy.text()).toContain('ws/nightly')
    expect(busy.text()).toContain('Queued #1')
  })

  it('shows each lane\'s capacity, even while nothing runs', async () => {
    state.lanes.value = [
      { lane: 'local', limit: 1, running: 0, waiting: 0, holder_keys: [] },
      { lane: 'cloud', limit: 3, running: 0, waiting: 0, holder_keys: [] },
    ]
    const w = await mountOverview()
    const meters = w.findAll('[role="meter"]').filter((m) => /lane$/.test(m.attributes('aria-label') ?? ''))
    expect(meters.map((m) => `${m.attributes('aria-label')} ${m.attributes('aria-valuenow')}/${m.attributes('aria-valuemax')}`)).toEqual(['Local lane 0/1', 'Cloud lane 0/3'])
    expect(w.text()).toContain('Nothing is running')
  })

  it('shows the lane error instead of an empty list when polling fails', async () => {
    state.laneError.value = 'connection refused'
    const w = await mountOverview()
    expect(w.text()).toContain('Could not read running work')
  })

  it('lists recent runs with outcome and formatted duration', async () => {
    const w = await mountOverview()
    const rows = w.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('ws/nightly')
    expect(rows[0]!.text()).toContain('Completed')
    expect(rows[0]!.text()).toContain('8.403s')
    expect(rows[1]!.text()).toContain('Failed')
  })

  it('links each recent run to its details', async () => {
    const w = await mountOverview()
    const a = w.findAll('tbody tr')[0]!.find('a')
    expect(a.attributes('href')).toBe('/activity?run=r-nightly')
    expect(a.attributes('aria-label')).toMatch(/^Open /)
  })

  it('reports a failed or empty run history', async () => {
    state.fetchGlobalActivity.mockImplementation(async () => {
      throw new Error('The server did not respond.')
    })
    expect((await mountOverview()).text()).toContain('The server did not respond.')
    state.fetchGlobalActivity.mockReset().mockResolvedValue([])
    expect((await mountOverview()).text()).toContain('No runs yet')
  })

  describe('running work links', () => {
    const since = '2026-09-29T10:00:00Z'
    const link = (w: Awaited<ReturnType<typeof mountOverview>>, name: RegExp) => w.findAll('a').find((a) => name.test(a.text()))

    it('links a running chat to its conversation', async () => {
      state.laneHolders.value = [{ key: 'chat:workspace-1', kind: 'interactive', workspace_id: 'workspace-1', label: 'chat:workspace-1', conversation_id: 'conv-42', since }]
      const w = await mountOverview()
      const a = link(w, /Chat in workspace-1/)
      expect(a, 'the running chat is a link').toBeDefined()
      expect(a!.attributes('href')).toBe('/workspaces/workspace-1/assistant/conv-42')
      expect(w.text()).not.toContain('chat:workspace-1')
    })

    it('links a running automation to its page', async () => {
      state.laneHolders.value = [{ key: 'workspace-1/nightly', kind: 'automation', workspace_id: 'workspace-1', automation: 'nightly', label: 'workspace-1/nightly', since }]
      const w = await mountOverview()
      const a = link(w, /workspace-1\/nightly/)
      expect(a, 'the running automation is a link').toBeDefined()
      expect(a!.attributes('href')).toContain('/automations/')
      expect(decodeURIComponent(a!.attributes('href')!)).toContain('workspace-1/nightly')
    })

    it('links a queued automation too', async () => {
      state.queuedRuns.value = [{ key: 'workspace-1/report', kind: 'automation', workspace_id: 'workspace-1', automation: 'report', label: 'workspace-1/report', position: 1, queued_at: since }]
      const w = await mountOverview()
      expect(link(w, /workspace-1\/report/)).toBeDefined()
    })

    it('shows an API caller as plain text, not a dead link', async () => {
      state.laneHolders.value = [{ key: 'inbound:1', kind: 'inbound', workspace_id: '', label: 'curl/8.4', since }]
      const w = await mountOverview()
      expect(w.text()).toContain('curl/8.4')
      expect(link(w, /curl\/8\.4/)).toBeUndefined()
    })

    it('tells the operator the row is a link', async () => {
      state.laneHolders.value = [{ key: 'chat:workspace-1', kind: 'interactive', workspace_id: 'workspace-1', label: 'chat:workspace-1', conversation_id: 'c', since }]
      const w = await mountOverview()
      expect(link(w, /Chat in workspace-1/)!.attributes('aria-label')).toMatch(/^Open /)
    })
  })
})
