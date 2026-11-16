import { describe, expect, it } from 'vitest'
import { byCloseness, changesOf, channelCounts, countFor, matchesFilter, narrow, strongestNow } from '../../../utils/radio/around'
import type { RadioLiveDevice } from '../../../types/radio'

const dev = (over: Partial<RadioLiveDevice>): RadioLiveDevice => ({
  id: 'aa', mine: false, class: 'neighbour', dbm: -70, near_m: 5, far_m: 20, ring: 'house', angle: 90, trend: 'steady', channel: 6, heard: true, ...over,
})

describe('around filters', () => {
  const list = [
    dev({ id: 'm', mine: true, class: 'personal' }),
    dev({ id: 'u', unusual: true, class: 'infrastructure' }),
    dev({ id: 'n', class: 'neighbour' }),
    dev({ id: 'x', class: 'neighbour', usual: true, heard: false }),
  ]

  it('matches everyone, the owner’s devices, the unusual ones and one kind of someone else’s', () => {
    expect(list.filter((d) => matchesFilter(d, 'all'))).toHaveLength(4)
    expect(list.filter((d) => matchesFilter(d, 'mine')).map((d) => d.id)).toEqual(['m'])
    expect(list.filter((d) => matchesFilter(d, 'unusual')).map((d) => d.id)).toEqual(['u'])
    expect(list.filter((d) => matchesFilter(d, 'neighbour')).map((d) => d.id)).toEqual(['n', 'x'])
    expect(matchesFilter(list[0]!, 'personal')).toBe(false) // a kind is only for someone else’s device
  })

  it('counts over every device, so a chip never says 0 for something the list would show', () => {
    expect(countFor(list, 'all')).toBe(4)
    expect(countFor(list, 'neighbour')).toBe(2)
  })

  it('narrows by chip and channel and keeps the ones usually here out unless asked', () => {
    const wide = [dev({ id: 'a', channel: 6 }), dev({ id: 'b', channel: 36 }), dev({ id: 'c', channel: 6, usual: true })]
    expect(narrow(wide, { filter: 'all', channel: 6, showUsual: false }).map((d) => d.id)).toEqual(['a'])
    expect(narrow(wide, { filter: 'all', channel: 6, showUsual: true }).map((d) => d.id)).toEqual(['a', 'c'])
    expect(narrow(wide, { filter: 'all', channel: null, showUsual: false }).map((d) => d.id)).toEqual(['a', 'b'])
  })

  it('puts the owner’s devices first, then the closest', () => {
    const sorted = byCloseness([dev({ id: 'far', near_m: 10, far_m: 40 }), dev({ id: 'mine', mine: true, near_m: 30, far_m: 60 }), dev({ id: 'near', near_m: 1, far_m: 3 })])
    expect(sorted.map((d) => d.id)).toEqual(['mine', 'near', 'far'])
  })
})

describe('what changed', () => {
  const detail = (first: string) => ({ id: 'z', class: 'neighbour' as const, why: 'w', signal: 'weak' as const, first_seen: first, last_seen: first, cycles_heard: 1, need_heard: 3, retry_percent: 0, channels: [6], random_address: false, joined_own: false })

  it('lists the unusual first, then new today, then the ones usually here and not heard, and skips the owner’s calm devices', () => {
    const out = changesOf([
      dev({ id: 'gone', usual: true, heard: false }),
      dev({ id: 'new', detail: detail('2026-11-15') }),
      dev({ id: 'odd', unusual: true }),
      dev({ id: 'old', detail: detail('2026-11-01') }),
      dev({ id: 'mine', mine: true }),
    ], '2026-11-15')
    expect(out.map((c) => `${c.kind}:${c.device.id}`)).toEqual(['unusual:odd', 'arrival:new', 'departure:gone'])
  })
})

describe('channels and the find-my-device shortlist', () => {
  it('counts the devices heard now on each channel, in channel order', () => {
    expect(channelCounts([dev({ channel: 60 }), dev({ channel: 6 }), dev({ channel: 60 }), dev({ channel: 11, usual: true })])).toEqual([{ channel: 6, devices: 1 }, { channel: 60, devices: 2 }])
  })

  it('offers the three strongest devices heard now that are not already the owner’s', () => {
    const out = strongestNow([dev({ id: 'a', dbm: -80 }), dev({ id: 'b', dbm: -40 }), dev({ id: 'm', mine: true, dbm: -30 }), dev({ id: 'c', dbm: -50 }), dev({ id: 'd', dbm: -60 }), dev({ id: 'u', dbm: -20, usual: true })])
    expect(out.map((d) => d.id)).toEqual(['b', 'c', 'd'])
  })
})
