import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { VulnDataStatus } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ readiness: vi.fn(), vulnData: vi.fn(), startVulnDownload: vi.fn(), cancelVulnDownload: vi.fn(), clearVulnData: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useVulnData } from '../../../composables/netaudit/useVulnData'

const source = { imported_at: '0001-01-01T00:00:00Z', cves: 0, entries: 0 }
const st = (over: Partial<VulnDataStatus>): VulnDataStatus => ({ state: 'idle', files_done: 0, files_total: 6, nvd: source, kev: source, ...over })

/** Each test is one consumer (a card on screen); stopping the scope is the card unmounting. */
function consumer() {
  const scope = effectScope()
  const api = scope.run(() => useVulnData())!
  return { api, unmount: () => scope.stop() }
}

describe('useVulnData', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    for (const fn of Object.values(service)) fn.mockReset()
    service.readiness.mockResolvedValue({ ready: true, requirements: [], notices: [] })
  })
  afterEach(() => vi.useRealTimers())

  it('reads the status once and does not poll while nothing runs', async () => {
    service.vulnData.mockResolvedValue(st({ state: 'idle' }))
    const { api, unmount } = consumer()
    await api.refresh()
    await vi.advanceTimersByTimeAsync(5000)
    expect(service.vulnData).toHaveBeenCalledTimes(1)
    unmount()
  })

  it('polls about once a second while a download runs, and stops when it ends', async () => {
    service.vulnData.mockResolvedValueOnce(st({ state: 'running', files_done: 1 })).mockResolvedValueOnce(st({ state: 'running', files_done: 2 })).mockResolvedValue(st({ state: 'done', files_done: 6 }))
    const { api, unmount } = consumer()
    await api.refresh()
    await vi.advanceTimersByTimeAsync(1000)
    expect(api.status.value?.files_done).toBe(2)
    await vi.advanceTimersByTimeAsync(1000)
    expect(api.status.value?.state).toBe('done')
    const calls = service.vulnData.mock.calls.length
    await vi.advanceTimersByTimeAsync(5000)
    expect(service.vulnData).toHaveBeenCalledTimes(calls)
    unmount()
  })

  it('re-reads what is missing for audits when a download ends, so the notice clears', async () => {
    service.vulnData.mockResolvedValueOnce(st({ state: 'running' })).mockResolvedValue(st({ state: 'done', files_done: 6 }))
    const { api, unmount } = consumer()
    await api.refresh()
    service.readiness.mockClear()
    await vi.advanceTimersByTimeAsync(1000)
    expect(service.readiness).toHaveBeenCalledTimes(1)
    unmount()
  })

  it('stops polling when the last consumer goes away', async () => {
    service.vulnData.mockResolvedValue(st({ state: 'running' }))
    const a = consumer()
    const b = consumer()
    await a.api.refresh()
    await vi.advanceTimersByTimeAsync(1000)
    a.unmount()
    const calls = service.vulnData.mock.calls.length
    await vi.advanceTimersByTimeAsync(1000)
    expect(service.vulnData.mock.calls.length).toBeGreaterThan(calls)
    b.unmount()
    const after = service.vulnData.mock.calls.length
    await vi.advanceTimersByTimeAsync(5000)
    expect(service.vulnData).toHaveBeenCalledTimes(after)
  })

  it('start shows the new state at once and then follows it', async () => {
    service.vulnData.mockResolvedValue(st({ state: 'idle' }))
    service.startVulnDownload.mockResolvedValue(st({ state: 'running' }))
    const { api, unmount } = consumer()
    await api.refresh()
    await api.start()
    expect(api.status.value?.state).toBe('running')
    service.vulnData.mockClear()
    await vi.advanceTimersByTimeAsync(1000)
    expect(service.vulnData).toHaveBeenCalled()
    unmount()
  })

  it('start can ask for every file again', async () => {
    service.startVulnDownload.mockResolvedValue(st({ state: 'running' }))
    const { api, unmount } = consumer()
    await api.start(true)
    expect(service.startVulnDownload).toHaveBeenCalledWith(true)
    await api.start()
    expect(service.startVulnDownload).toHaveBeenLastCalledWith(false)
    unmount()
  })

  it('says why a download could not start and keeps the screen as it was', async () => {
    service.vulnData.mockResolvedValue(st({ state: 'running' }))
    service.startVulnDownload.mockRejectedValue(new Error('a download is already running'))
    const { api, unmount } = consumer()
    await api.refresh()
    await api.start()
    expect(api.actionError.value).toContain('already running')
    expect(api.status.value?.state).toBe('running')
    unmount()
  })

  it('cancel asks the server to stop and shows what it answers', async () => {
    service.vulnData.mockResolvedValue(st({ state: 'running' }))
    service.cancelVulnDownload.mockResolvedValue(st({ state: 'stopped', files_done: 2, reason: 'Cancelled.' }))
    const { api, unmount } = consumer()
    await api.refresh()
    await api.cancel()
    expect(api.status.value?.state).toBe('stopped')
    unmount()
  })

  it('clear deletes the data, shows the empty status and re-reads what audits are missing', async () => {
    service.clearVulnData.mockResolvedValue(st({ state: 'idle', files_done: 0 }))
    const { api, unmount } = consumer()
    await api.clear()
    expect(service.clearVulnData).toHaveBeenCalledTimes(1)
    expect(api.status.value?.files_done).toBe(0)
    expect(service.readiness).toHaveBeenCalled()
    unmount()
  })

  it('says why the data could not be deleted', async () => {
    service.clearVulnData.mockRejectedValue(new Error('read-only'))
    const { api, unmount } = consumer()
    await api.clear()
    expect(api.actionError.value).toBe('read-only')
    unmount()
  })

  it('says the status could not be read', async () => {
    service.vulnData.mockRejectedValue(new Error('boom'))
    const { api, unmount } = consumer()
    await api.refresh()
    expect(api.error.value).toBe('boom')
    unmount()
  })
})
