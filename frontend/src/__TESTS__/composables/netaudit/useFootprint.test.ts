import { beforeEach, describe, expect, it, vi } from 'vitest'

const service = vi.hoisted(() => ({ footprint: vi.fn(), changeFootprint: vi.fn(), setFootprintKey: vi.fn(), clearFootprintKey: vi.fn(), checkFootprint: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useFootprint } from '../../../composables/netaudit/useFootprint'

const page = (name: string) => ({ watches: [{ label: `domain:${name}`, name, kind: 'domain', frequency: 'manual', results: [] }], services: [], key_saved: false, max_domains: 10, max_emails: 10 })

describe('useFootprint', () => {
  beforeEach(() => Object.values(service).forEach((f) => f.mockReset()))

  it('replaces the page with the answer of each change, and says why a refused change failed', async () => {
    service.footprint.mockResolvedValue(page('a.com'))
    service.changeFootprint.mockResolvedValueOnce(page('b.com')).mockRejectedValueOnce(new Error('at most 10 domains can be watched'))
    const f = useFootprint()
    await f.load()
    expect(f.state.value?.watches[0]?.name).toBe('a.com')
    expect(await f.apply({ action: 'add_domain', name: 'b.com' })).toBe(true)
    expect(f.state.value?.watches[0]?.name).toBe('b.com')
    expect(await f.apply({ action: 'add_domain', name: 'c.com' })).toBe(false)
    expect(f.error.value).toContain('at most 10')
  })

  it('checks one watch at a time and trims the key', async () => {
    let release: (v: unknown) => void = () => {}
    service.checkFootprint.mockReturnValue(new Promise((res) => { release = res }))
    service.setFootprintKey.mockResolvedValue(page('a.com'))
    const f = useFootprint()
    const first = f.check('domain:a.com')
    await f.check('domain:b.com')
    expect(service.checkFootprint).toHaveBeenCalledTimes(1)
    release(page('a.com'))
    await first
    expect(f.checking.value).toBe('')
    await f.saveKey('  k1  ')
    expect(service.setFootprintKey).toHaveBeenCalledWith('k1')
  })
})
