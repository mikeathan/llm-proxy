import { beforeEach, describe, expect, it, vi } from 'vitest'

const service = vi.hoisted(() => ({
  routerConn: vi.fn(), setRouterTarget: vi.fn(), setRouterKey: vi.fn(), deleteRouterKey: vi.fn(), routerFingerprint: vi.fn(), pinRouter: vi.fn(), readRouter: vi.fn(),
}))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useRouterConnections } from '../../../composables/netaudit/useRouterConnections'

describe('useRouterConnections', () => {
  beforeEach(() => Object.values(service).forEach((f) => f.mockReset()))

  it('names the next setup step from what is saved', async () => {
    const c = useRouterConnections()
    expect(c.step.value).toBe('target')
    for (const [state, step] of [
      [{ address: '192.168.50.1', user: '', key_saved: false }, 'target'],
      [{ address: '192.168.50.1', user: 'admin', key_saved: false }, 'key'],
      [{ address: '192.168.50.1', user: 'admin', key_saved: true }, 'trust'],
      [{ address: '192.168.50.1', user: 'admin', key_saved: true, fingerprint: 'SHA256:x' }, 'ready'],
    ] as const) {
      service.routerConn.mockResolvedValue(state)
      await c.load()
      expect(c.step.value).toBe(step)
    }
  })

  it('does one thing at a time, and keeps the server\'s words when a step is refused', async () => {
    const c = useRouterConnections()
    let release: (v: unknown) => void = () => {}
    service.readRouter.mockReturnValue(new Promise((r) => { release = r }))
    const first = c.read()
    expect(c.busy.value).toBe('read')
    expect(await c.read()).toBe(false)
    expect(service.readRouter).toHaveBeenCalledTimes(1)
    release({ address: 'a', user: 'u', key_saved: true })
    await first
    service.readRouter.mockRejectedValue(new Error('the router was just read; wait a few seconds'))
    expect(await c.read()).toBe(false)
    expect(c.error.value).toContain('just read')
    expect(c.busy.value).toBe('')
  })

  it('trusts only the fingerprint it was shown, and forgets it afterwards', async () => {
    const c = useRouterConnections()
    service.routerFingerprint.mockResolvedValue({ fingerprint: 'SHA256:abc' })
    await c.check()
    expect(c.candidate.value).toBe('SHA256:abc')
    service.pinRouter.mockResolvedValue({ address: 'a', user: 'u', key_saved: true, fingerprint: 'SHA256:abc' })
    await c.trust()
    expect(service.pinRouter).toHaveBeenCalledWith('SHA256:abc')
    expect(c.candidate.value).toBe('')
  })
})
