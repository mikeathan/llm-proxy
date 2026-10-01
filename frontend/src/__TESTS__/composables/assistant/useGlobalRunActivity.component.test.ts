import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const getGlobalActiveRunsMock = vi.fn()
vi.mock('../../../services/assistant/assistantService', () => ({
  AssistantService: { getGlobalActiveRuns: (...args: unknown[]) => getGlobalActiveRunsMock(...args) },
}))

const POLL_MS = 10_000
const HOLDER = { key: 'ws/a', kind: 'automation', workspace_id: 'ws', automation: 'a', label: 'ws/a', since: '2026-09-28T10:00:00Z' }

// Fresh module per test: the composable is a module-level singleton.
async function load() {
  vi.resetModules()
  return (await import('../../../composables/assistant/useGlobalRunActivity')).useGlobalRunActivity
}

function setVisibility(state: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state })
  document.dispatchEvent(new Event('visibilitychange'))
}

// Each fresh module copy registers its own visibility listener on the shared
// document; remove them after every test so old copies do not poll.
const visibilityListeners: EventListenerOrEventListenerObject[] = []
const addListener = document.addEventListener.bind(document)
vi.spyOn(document, 'addEventListener').mockImplementation((type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions) => {
  if (type === 'visibilitychange') visibilityListeners.push(listener)
  addListener(type, listener, options)
})

describe('useGlobalRunActivity polling', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getGlobalActiveRunsMock.mockReset().mockResolvedValue({ lane_holders: [HOLDER], queued: [] })
    setVisibility('visible')
  })
  afterEach(() => {
    visibilityListeners.splice(0).forEach((l) => document.removeEventListener('visibilitychange', l))
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  // Characterisation (plan D22): one shared interval however many consumers.
  it('keeps a single interval for any number of consumers', async () => {
    const intervals = vi.spyOn(globalThis, 'setInterval')
    const useGlobalRunActivity = await load()
    useGlobalRunActivity()
    useGlobalRunActivity()
    expect(intervals).toHaveBeenCalledTimes(1)
    intervals.mockRestore()
  })

  it('records each tick with its status, keeping the last good snapshot on a failed one', async () => {
    const useGlobalRunActivity = await load()
    const { lastTick, laneHolders } = useGlobalRunActivity()
    await vi.waitFor(() => expect(lastTick.value?.status).toBe('ok'))
    const first = lastTick.value!.seq
    expect(lastTick.value!.holders).toEqual([HOLDER])

    getGlobalActiveRunsMock.mockRejectedValueOnce(new Error('down'))
    await vi.advanceTimersByTimeAsync(POLL_MS)
    expect(lastTick.value).toMatchObject({ seq: first + 1, status: 'error' })
    expect(laneHolders.value).toEqual([HOLDER])
  })

  it('pauses on a hidden tab and ticks immediately when visible again', async () => {
    const useGlobalRunActivity = await load()
    useGlobalRunActivity()
    await vi.advanceTimersByTimeAsync(0)
    const callsBefore = getGlobalActiveRunsMock.mock.calls.length

    setVisibility('hidden')
    await vi.advanceTimersByTimeAsync(POLL_MS * 3)
    expect(getGlobalActiveRunsMock.mock.calls.length).toBe(callsBefore)

    setVisibility('visible')
    await vi.advanceTimersByTimeAsync(0)
    expect(getGlobalActiveRunsMock.mock.calls.length).toBe(callsBefore + 1)
    await vi.advanceTimersByTimeAsync(POLL_MS)
    expect(getGlobalActiveRunsMock.mock.calls.length).toBe(callsBefore + 2)
  })
})
