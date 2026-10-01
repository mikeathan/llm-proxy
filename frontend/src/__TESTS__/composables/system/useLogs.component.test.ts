import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, KeepAlive, nextTick, ref } from 'vue'

const fetchLogsMock = vi.fn()
vi.mock('../../../services/monitoring/metricsService', () => ({
  MetricsApiService: {
    fetchLogs: () => fetchLogsMock(),
    fetchAppLogs: () => Promise.resolve({ logs: '' }),
    clearLogs: vi.fn(),
    clearAppLogs: vi.fn(),
  },
}))

const POLL_MS = 10_000

describe('useLogs polling lifecycle', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.resetModules()
    fetchLogsMock.mockReset().mockResolvedValue({ running: false, logs: '' })
  })
  afterEach(() => vi.useRealTimers())

  it('stops polling while its kept-alive view is in the background and resumes on return', async () => {
    const { useLogs } = await import('../../../composables/system/useLogs')
    const Viewer = defineComponent({ name: 'Viewer', setup() { useLogs(); return () => h('div') } })
    const shown = ref(true)
    mount(defineComponent({ render: () => h(KeepAlive, null, [shown.value ? h(Viewer) : h('p')]) }))
    await vi.advanceTimersByTimeAsync(POLL_MS)
    const whileShown = fetchLogsMock.mock.calls.length
    expect(whileShown).toBeGreaterThanOrEqual(2)

    shown.value = false
    await nextTick()
    await vi.advanceTimersByTimeAsync(POLL_MS * 3)
    expect(fetchLogsMock.mock.calls.length).toBe(whileShown)

    shown.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(0)
    expect(fetchLogsMock.mock.calls.length).toBe(whileShown + 1)
  })
})
