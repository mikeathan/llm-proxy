import { beforeEach, describe, expect, it, vi } from 'vitest'

const service = vi.hoisted(() => ({ recheck: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useRecheck } from '../../../composables/netaudit/useRecheck'

describe('useRecheck', () => {
  beforeEach(() => service.recheck.mockReset())

  it('asks the server about one finding, keeps the answer by key and reloads the report', async () => {
    service.recheck.mockResolvedValue({ outcome: 'fixed', reason: 'gone', checked_at: 'now' })
    const reload = vi.fn().mockResolvedValue(undefined)
    const r = useRecheck(() => 'ws', () => 'nightly', reload)
    await r.run('k1')
    expect(service.recheck).toHaveBeenCalledWith({ workspace_id: 'ws', automation: 'nightly', key: 'k1', note: '' })
    expect(r.results.k1?.outcome).toBe('fixed')
    expect(reload).toHaveBeenCalledOnce()
    expect(r.busyKey.value).toBe('')
  })

  it('says why a recheck failed and never records an answer for it', async () => {
    service.recheck.mockRejectedValueOnce(new Error('A scan is running. Try again when it has finished.'))
    const r = useRecheck(() => 'ws', () => 'nightly', vi.fn())
    await r.run('k1')
    expect(r.errors.k1).toContain('A scan is running')
    expect(r.results.k1).toBeUndefined()
    expect(r.busyKey.value).toBe('')
  })

  it('checks one finding at a time', async () => {
    let release: (v: unknown) => void = () => {}
    service.recheck.mockReturnValue(new Promise((res) => { release = res }))
    const r = useRecheck(() => 'ws', () => 'nightly', vi.fn().mockResolvedValue(undefined))
    const first = r.run('k1')
    await r.run('k2')
    expect(service.recheck).toHaveBeenCalledTimes(1)
    release({ outcome: 'still_there', reason: '', checked_at: '' })
    await first
  })
})
