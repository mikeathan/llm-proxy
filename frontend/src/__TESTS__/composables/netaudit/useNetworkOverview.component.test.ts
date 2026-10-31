import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { NetworkAudit, NetworkOverview } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ overview: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useNetworkOverview } from '../../../composables/netaudit/useNetworkOverview'
import { NETWORK_REFRESH_MS } from '../../../constants/netaudit'

const OVERVIEW = { posture: 'quiet' } as NetworkOverview

function setVisibility(state: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state })
}

describe('useNetworkOverview', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    service.overview.mockReset()
    service.overview.mockResolvedValue(OVERVIEW)
    setVisibility('visible')
  })
  afterEach(() => {
    vi.useRealTimers()
    setVisibility('visible')
  })

  it('reads once on start and then on every interval while the page is visible', async () => {
    const o = useNetworkOverview(ref<NetworkAudit | null>(null))
    o.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(o.overview.value).toEqual(OVERVIEW)
    await vi.advanceTimersByTimeAsync(NETWORK_REFRESH_MS * 2)
    expect(service.overview).toHaveBeenCalledTimes(3)
    o.stop()
  })

  it('does not poll a hidden tab, and reads again as soon as it is shown', async () => {
    const o = useNetworkOverview(ref<NetworkAudit | null>(null))
    o.start()
    await vi.advanceTimersByTimeAsync(0)
    setVisibility('hidden')
    await vi.advanceTimersByTimeAsync(NETWORK_REFRESH_MS * 3)
    expect(service.overview).toHaveBeenCalledTimes(1)
    setVisibility('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(0)
    expect(service.overview).toHaveBeenCalledTimes(2)
    o.stop()
  })

  it('stops for good: no timer and no listener are left behind', async () => {
    const o = useNetworkOverview(ref<NetworkAudit | null>(null))
    o.start()
    await vi.advanceTimersByTimeAsync(0)
    o.stop()
    await vi.advanceTimersByTimeAsync(NETWORK_REFRESH_MS * 3)
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(0)
    expect(service.overview).toHaveBeenCalledTimes(1)
  })

  it('never has two reads in flight, and reads once more when asked again meanwhile', async () => {
    let finish: (v: NetworkOverview) => void = () => {}
    service.overview.mockReturnValueOnce(new Promise<NetworkOverview>((r) => (finish = r)))
    const o = useNetworkOverview(ref<NetworkAudit | null>(null))
    void o.load()
    void o.load()
    void o.load()
    expect(service.overview).toHaveBeenCalledTimes(1)
    finish(OVERVIEW)
    await vi.advanceTimersByTimeAsync(0)
    expect(service.overview).toHaveBeenCalledTimes(2)
  })

  it('asks for the audit it was given, and keeps the last overview when a refresh fails', async () => {
    const audit = ref<NetworkAudit | null>({ workspaceId: 'ws', automation: 'nightly' })
    const o = useNetworkOverview(audit)
    await o.load()
    expect(service.overview).toHaveBeenCalledWith({ workspaceId: 'ws', automation: 'nightly' })
    service.overview.mockRejectedValue(new Error('down'))
    await o.load()
    expect(o.overview.value).toEqual(OVERVIEW)
    expect(o.error.value).toContain('down')
  })
})
