import { describe, it, expect, vi, beforeEach } from 'vitest'

const getGlobalActiveRunsMock = vi.fn()

vi.mock('../../../services/assistant/assistantService', () => ({
  AssistantService: {
    getGlobalActiveRuns: (...args: unknown[]) => getGlobalActiveRunsMock(...args),
  },
}))

import { useGlobalRunActivity } from '../../../composables/assistant/useGlobalRunActivity'

describe('useGlobalRunActivity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('exposes the global lane state without workspace context', async () => {
    getGlobalActiveRunsMock.mockResolvedValue({
      lane_holders: [
        { key: 'ws/a', kind: 'automation', workspace_id: 'ws', automation: 'a', label: 'ws/a', since: '2026-09-20T00:00:00Z' },
        { key: 'other/chat', kind: 'interactive', workspace_id: 'other', label: 'other', since: '2026-09-20T00:00:00Z' },
      ],
      queued: [
        { key: 'ws/b', lane: 'cloud', workspace_id: 'ws', automation: 'b', label: 'ws/b', position: 2, queued_at: '2026-09-20T00:00:00Z' },
      ],
    })

    const { laneHolders, queuedRuns, runningCount, queuedCount, isActive } = useGlobalRunActivity()

    await vi.waitFor(() => {
      expect(runningCount.value).toBe(2)
    })
    expect(laneHolders.value[0]?.label).toBe('ws/a')
    expect(queuedCount.value).toBe(1)
    expect(queuedRuns.value[0]?.position).toBe(2)
    expect(isActive.value).toBe(true)
    // The global endpoint takes no workspace argument.
    expect(getGlobalActiveRunsMock).toHaveBeenCalledWith()
  })

  it('treats a payload with no lane entries as idle', async () => {
    getGlobalActiveRunsMock.mockResolvedValue({})

    const { laneHolders, queuedRuns, runningCount, queuedCount, isActive, refresh } = useGlobalRunActivity()

    await refresh()

    expect(laneHolders.value).toEqual([])
    expect(queuedRuns.value).toEqual([])
    expect(runningCount.value).toBe(0)
    expect(queuedCount.value).toBe(0)
    expect(isActive.value).toBe(false)
  })

  it('surfaces the failure instead of throwing when the request fails', async () => {
    getGlobalActiveRunsMock.mockRejectedValue(new Error('boom'))

    const { error, refresh, isActive } = useGlobalRunActivity()

    await expect(refresh()).resolves.toBeUndefined()
    expect(error.value).toBe('boom')
    expect(isActive.value).toBe(false)
  })
})
