import { describe, expect, it } from 'vitest'
import { anyFlagged, flagWords, groupLookups } from '../../../utils/netaudit/lookups'
import type { DeviceLookupName, DeviceLookups } from '../../../types/netaudit'

const name = (n: string, domain: string, last: string, flag?: DeviceLookupName['flag']): DeviceLookupName => ({ name: n, domain, last, count: 1, flag })

describe('groupLookups', () => {
  it('groups names by domain, the newest group first, with the most serious flag of the group', () => {
    const groups = groupLookups([
      name('a.apple.com', 'apple.com', '2026-11-11T10:00:00Z'),
      name('x.googleapis.com', 'googleapis.com', '2026-11-11T12:00:00Z', 'new_place'),
      name('b.apple.com', 'apple.com', '2026-11-11T13:00:00Z', 'known_bad'),
      name('c.apple.com', 'apple.com', '2026-11-11T09:00:00Z', 'new_place'),
    ])
    expect(groups.map((g) => g.domain)).toEqual(['apple.com', 'googleapis.com'])
    expect(groups[0]).toMatchObject({ last: '2026-11-11T13:00:00Z', flag: 'known_bad' })
    expect(groups[0]!.names.map((n) => n.name)).toEqual(['a.apple.com', 'b.apple.com', 'c.apple.com'])
  })

  it('falls back to the name when the server gave no domain', () => {
    expect(groupLookups([name('printer', '', '2026-11-11T10:00:00Z')])[0]!.domain).toBe('printer')
  })
})

describe('flagWords', () => {
  it('puts the most serious flag first, in words', () => {
    expect(flagWords(['new_place', 'known_bad']).map((f) => f.label)).toEqual(['Known malware host', 'New place'])
  })
  it('says whether any device raised a flag', () => {
    const d = (flags: DeviceLookups['flags']): DeviceLookups => ({ mac: 'aa', first: '', last: '', today: 0, total: 0, flags, names: [] })
    expect(anyFlagged([d([]), d(['beaconing'])])).toBe(true)
    expect(anyFlagged([d([])])).toBe(false)
  })
})
