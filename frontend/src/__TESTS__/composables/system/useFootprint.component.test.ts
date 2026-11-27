import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'

const fetchFootprint = vi.fn()
vi.mock('../../../services/monitoring/metricsService', () => ({ MetricsApiService: { fetchFootprint: () => fetchFootprint() } }))

const FP = { processes: [], go: { heap_mb: 1, total_mb: 2, goroutines: 3, gc_cycles: 4 }, timestamp: 'now' }

async function setup(initiallyOpen = false) {
  const { useFootprint, FOOTPRINT_POLL_MS } = await import('../../../composables/system/useFootprint')
  const open = ref(initiallyOpen)
  const scope = effectScope()
  const api = scope.run(() => useFootprint(open))!
  return { open, scope, api, FOOTPRINT_POLL_MS }
}

describe('useFootprint', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    fetchFootprint.mockReset()
    fetchFootprint.mockResolvedValue(FP)
  })
  afterEach(() => vi.useRealTimers())

  it('reads nothing while the panel is closed', async () => {
    const { scope, FOOTPRINT_POLL_MS } = await setup(false)
    await vi.advanceTimersByTimeAsync(FOOTPRINT_POLL_MS * 3)
    expect(fetchFootprint).not.toHaveBeenCalled()
    scope.stop()
  })

  it('reads when the panel opens, then on the interval, and stops when it closes', async () => {
    const { open, scope, api, FOOTPRINT_POLL_MS } = await setup(false)
    open.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(0)
    expect(fetchFootprint).toHaveBeenCalledTimes(1)
    expect(api.footprint.value).toEqual(FP)
    await vi.advanceTimersByTimeAsync(FOOTPRINT_POLL_MS)
    expect(fetchFootprint).toHaveBeenCalledTimes(2)
    open.value = false
    await nextTick()
    await vi.advanceTimersByTimeAsync(FOOTPRINT_POLL_MS * 3)
    expect(fetchFootprint).toHaveBeenCalledTimes(2)
    scope.stop()
  })

  it('remembers a failed read without throwing, and recovers on the next one', async () => {
    fetchFootprint.mockRejectedValueOnce(new Error('down'))
    const { scope, api, FOOTPRINT_POLL_MS } = await setup(true)
    await vi.advanceTimersByTimeAsync(0)
    expect(api.failed.value).toBe(true)
    await vi.advanceTimersByTimeAsync(FOOTPRINT_POLL_MS)
    expect(api.failed.value).toBe(false)
    expect(api.footprint.value).toEqual(FP)
    scope.stop()
  })

  it('stops the poll when its scope ends', async () => {
    const { scope, FOOTPRINT_POLL_MS } = await setup(true)
    await vi.advanceTimersByTimeAsync(0)
    scope.stop()
    await vi.advanceTimersByTimeAsync(FOOTPRINT_POLL_MS * 3)
    expect(fetchFootprint).toHaveBeenCalledTimes(1)
  })
})
