import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, KeepAlive, nextTick, ref } from 'vue'

// Phase 6 leak audit: a poller owned by a kept-alive view must stop while the
// view is in the background, poll again at once when it returns, and stop for
// good when the view unmounts.
const getActiveRuns = vi.fn()
vi.mock('../../../services/assistant/assistantService', () => ({ AssistantService: { getActiveRuns: (ws: string) => getActiveRuns(ws) } }))
const fetchMetrics = vi.fn()
vi.mock('../../../services/monitoring/metricsService', () => ({
  MetricsApiService: { fetchMetrics: () => fetchMetrics(), fetchLogLevel: vi.fn().mockResolvedValue('INFO'), updateLogLevel: vi.fn() },
}))

const POLL = 10_000

// Mounts `owner` in a KeepAlive next to another view; `show` switches between them.
async function keptAlive(owner: ReturnType<typeof defineComponent>) {
  const show = ref(true)
  const other = defineComponent({ name: 'Other', render: () => h('p', 'elsewhere') })
  const w = mount(defineComponent({ render: () => h(KeepAlive, null, [show.value ? h(owner) : h(other)]) }))
  await nextTick()
  return { w, show }
}

async function scenario(owner: ReturnType<typeof defineComponent>, calls: () => number) {
  const { w, show } = await keptAlive(owner)
  const onMount = calls()
  expect(onMount).toBeGreaterThanOrEqual(1)

  await vi.advanceTimersByTimeAsync(POLL)
  expect(calls()).toBe(onMount + 1)

  show.value = false // the view goes to the background
  await nextTick()
  await vi.advanceTimersByTimeAsync(POLL * 3)
  expect(calls()).toBe(onMount + 1)

  show.value = true // back on screen: fresh data at once, then the interval
  await nextTick()
  expect(calls()).toBe(onMount + 2)
  await vi.advanceTimersByTimeAsync(POLL)
  expect(calls()).toBe(onMount + 3)

  w.unmount()
  await vi.advanceTimersByTimeAsync(POLL * 3)
  expect(calls()).toBe(onMount + 3)
}

describe('poller lifecycle', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.useFakeTimers()
    getActiveRuns.mockReset().mockResolvedValue({ assistant_running: false, automation_running: false })
    fetchMetrics.mockReset().mockResolvedValue({})
  })
  afterEach(() => vi.useRealTimers())

  it('useRunningActivity polls only while its view is on screen', async () => {
    const { useRunningActivity } = await import('../../../composables/assistant/useRunningActivity')
    const owner = defineComponent({ name: 'Owner', setup: () => (useRunningActivity(ref('ws')), () => h('p', 'ws')) })
    await scenario(owner, () => getActiveRuns.mock.calls.length)
  })

  it('useMetrics polls only while its view is on screen', async () => {
    const { useMetrics } = await import('../../../composables/system/useMetrics')
    const owner = defineComponent({ name: 'Owner', setup: () => (useMetrics(), () => h('p', 'metrics')) })
    await scenario(owner, () => fetchMetrics.mock.calls.length)
  })

  describe('useMetrics on a hidden tab', () => {
    const setVisibility = (state: 'hidden' | 'visible') => {
      Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state })
      document.dispatchEvent(new Event('visibilitychange'))
    }
    afterEach(() => setVisibility('visible'))

    it('stops polling while the tab is hidden and refreshes at once on return', async () => {
      const { useMetrics } = await import('../../../composables/system/useMetrics')
      const owner = defineComponent({ name: 'Owner', setup: () => (useMetrics(), () => h('p', 'metrics')) })
      const w = mount(owner)
      await nextTick()
      const onMount = fetchMetrics.mock.calls.length
      await vi.advanceTimersByTimeAsync(POLL)
      expect(fetchMetrics.mock.calls.length).toBe(onMount + 1)

      setVisibility('hidden')
      await vi.advanceTimersByTimeAsync(POLL * 5)
      expect(fetchMetrics.mock.calls.length).toBe(onMount + 1)

      setVisibility('visible')
      await vi.advanceTimersByTimeAsync(0)
      expect(fetchMetrics.mock.calls.length).toBe(onMount + 2)
      await vi.advanceTimersByTimeAsync(POLL)
      expect(fetchMetrics.mock.calls.length).toBe(onMount + 3)
      w.unmount()
    })

    it('does not start polling when a consumer mounts on a hidden tab, and stays stopped after unmount', async () => {
      const { useMetrics } = await import('../../../composables/system/useMetrics')
      setVisibility('hidden')
      const owner = defineComponent({ name: 'Owner', setup: () => (useMetrics(), () => h('p', 'metrics')) })
      const w = mount(owner)
      await nextTick()
      const onMount = fetchMetrics.mock.calls.length
      await vi.advanceTimersByTimeAsync(POLL * 3)
      expect(fetchMetrics.mock.calls.length).toBe(onMount)

      w.unmount()
      setVisibility('visible') // nobody is listening any more: no poll may start
      await vi.advanceTimersByTimeAsync(POLL * 3)
      expect(fetchMetrics.mock.calls.length).toBe(onMount)
    })
  })
})
