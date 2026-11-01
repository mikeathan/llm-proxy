import { describe, expect, it } from 'vitest'
import { outsideLookupLine, outsideLookupNeedsAddress } from '../../../utils/netaudit/outsideLookup'
import { OUTSIDE_LOOKUP_NOT_YET, OUTSIDE_LOOKUP_REASON_OTHER, OUTSIDE_LOOKUP_UNKNOWN } from '../../../constants/netaudit'

const last = (over: object) => ({ at: '2026-11-01T02:00:00Z', ports: 0, ...over })

describe('outsideLookupLine', () => {
  it('says nothing was tried yet when there is no outcome', () => {
    expect(outsideLookupLine(last({}), '')).toBe(OUTSIDE_LOOKUP_NOT_YET)
  })

  it('counts the ports a look found, singular and plural', () => {
    expect(outsideLookupLine(last({ outcome: 'looked', ports: 1 }), '3 hours ago')).toContain('1 open port on your address')
    expect(outsideLookupLine(last({ outcome: 'looked', ports: 3 }), '3 hours ago')).toContain('3 open ports')
    expect(outsideLookupLine(last({ outcome: 'looked', ports: 0 }), '3 hours ago')).toContain('lists no open ports')
  })

  it('never reads a look that could not be made as "nothing is open"', () => {
    for (const reason of ['router_not_asked', 'upnp_off', 'no_address', 'private_address', 'shared_address', 'link_local_address']) {
      const line = outsideLookupLine(last({ outcome: 'cannot_look', reason }), 'now')
      expect(line).toContain('Nothing was sent')
      expect(line).toContain(OUTSIDE_LOOKUP_UNKNOWN)
    }
    expect(outsideLookupLine(last({ outcome: 'failed', reason: 'lookup_failed' }), 'now')).toContain(OUTSIDE_LOOKUP_UNKNOWN)
  })

  it('says internet access is off instead of the service being down', () => {
    const line = outsideLookupLine(last({ outcome: 'failed', reason: 'internet_off' }), 'now')
    expect(line).toContain('internet access is off')
    expect(line).not.toContain('could not be reached')
    expect(line).toContain(OUTSIDE_LOOKUP_UNKNOWN)
  })

  it('has words for a reason code this version does not know', () => {
    expect(outsideLookupLine(last({ outcome: 'cannot_look', reason: 'brand_new' }), 'now')).toContain(OUTSIDE_LOOKUP_REASON_OTHER)
  })
})

describe('outsideLookup address', () => {
  it('says a look used the address that was entered', () => {
    expect(outsideLookupLine(last({ outcome: 'looked', ports: 1, source: 'entered' }), '3 hours ago')).toContain('at the address you entered')
    expect(outsideLookupLine(last({ outcome: 'looked', ports: 1, source: 'router' }), '3 hours ago')).not.toContain('entered')
  })
  it('asks for an address only when the router could not give a usable one', () => {
    for (const reason of ['upnp_off', 'no_address', 'private_address']) expect(outsideLookupNeedsAddress(last({ outcome: 'cannot_look', reason }))).toBe(true)
    for (const reason of ['shared_address', 'link_local_address', 'internet_off', 'router_not_asked']) expect(outsideLookupNeedsAddress(last({ outcome: 'cannot_look', reason }))).toBe(false)
    expect(outsideLookupNeedsAddress(last({ outcome: 'looked' }))).toBe(false)
    expect(outsideLookupNeedsAddress(last({}))).toBe(false)
  })
})
