import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'

const service = vi.hoisted(() => ({ nearby: vi.fn(), setNearby: vi.fn(), nearbyLook: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useNearby } from '../../../composables/netaudit/useNearby'

const running = { enabled: true, running: true }
const idle = { enabled: true, running: false }

describe('useNearby', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    service.nearby.mockReset()
    service.nearbyLook.mockReset()
  })
  afterEach(() => vi.useRealTimers())

  it('reads the state once a second while a look runs, and stops when it ends', async () => {
    service.nearby.mockResolvedValueOnce(running).mockResolvedValueOnce(running).mockResolvedValue(idle)
    const scope = effectScope()
    const nearby = scope.run(() => useNearby())!
    await nearby.load()
    await vi.advanceTimersByTimeAsync(1000)
    await vi.advanceTimersByTimeAsync(1000)
    await vi.advanceTimersByTimeAsync(5000)
    expect(service.nearby).toHaveBeenCalledTimes(3)
    scope.stop()
  })

  it('never polls again after its owner is gone, even when an answer arrives afterwards', async () => {
    let answer!: (v: unknown) => void
    service.nearbyLook.mockReturnValue(new Promise((resolve) => { answer = resolve }))
    service.nearby.mockResolvedValue(running)
    const scope = effectScope()
    const nearby = scope.run(() => useNearby())!
    const asked = nearby.check()
    scope.stop() // the card is closed while the server is still answering
    answer(running)
    await asked
    await vi.advanceTimersByTimeAsync(10_000)
    expect(service.nearby).not.toHaveBeenCalled()
  })
})
