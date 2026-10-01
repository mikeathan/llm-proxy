import { describe, it, expect, vi, beforeEach } from 'vitest'

const fetchMetricsMock = vi.fn()
const updateLogLevelMock = vi.fn()
vi.mock('../../../services/monitoring/metricsService', () => ({
  MetricsApiService: { fetchMetrics: () => fetchMetricsMock(), fetchLogLevel: vi.fn(), updateLogLevel: (l: string) => updateLogLevelMock(l) },
}))
const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn() }
vi.mock('../../../composables/useToast', () => ({ useToast: () => toast }))

// refresh() and history are module-level (the poll's owner mounts; these tests
// drive refresh directly, so no interval is started). Fresh module per test.
async function load() {
  vi.resetModules()
  const { useMetrics } = await import('../../../composables/system/useMetrics')
  return useMetrics()
}

describe('useMetrics history (plan V8)', () => {
  beforeEach(() => vi.clearAllMocks())

  it('keeps a capped history of throughput and CPU load, one sample per poll, oldest first', async () => {
    const api = await load()
    for (let i = 0; i < 35; i++) {
      fetchMetricsMock.mockResolvedValueOnce({ load_percent: i, mem_used_mb: 1, mem_total_mb: 2, llm_tokens_per_sec: i * 2 })
      await api.refresh()
    }
    expect(api.history.value).toHaveLength(30)
    expect(api.history.value[0]).toEqual({ tokensPerSecond: 10, loadPercent: 5 })
    expect(api.history.value[29]).toEqual({ tokensPerSecond: 68, loadPercent: 34 })
  })

  it('does not record a sample for a failed poll', async () => {
    const api = await load()
    fetchMetricsMock.mockRejectedValue(new Error('down'))
    await expect(api.refresh()).resolves.toBeUndefined()
    expect(api.history.value).toEqual([])
  })
})

describe('useLogLevel', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reports a failed change as an error toast, not a browser alert, and keeps the old level', async () => {
    vi.resetModules()
    const alertSpy = vi.fn()
    vi.stubGlobal('alert', alertSpy)
    updateLogLevelMock.mockRejectedValue(new Error('forbidden'))
    const { useLogLevel } = await import('../../../composables/system/useMetrics')
    const { logLevel, updateLogLevel } = useLogLevel()
    const before = logLevel.value
    await updateLogLevel('DEBUG')
    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('forbidden'))
    expect(alertSpy).not.toHaveBeenCalled()
    expect(logLevel.value).toBe(before)
    vi.unstubAllGlobals()
  })
})
