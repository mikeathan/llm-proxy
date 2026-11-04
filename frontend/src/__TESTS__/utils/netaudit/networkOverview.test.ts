import { describe, expect, it } from 'vitest'
import { attentionReasons, changeText, hasRouterCounts, nextAction, postureHeadline, refusedToday, seriesOf } from '../../../utils/netaudit/networkOverview'
import type { NetworkDay, NetworkOverview } from '../../../types/netaudit'

const overview = (over: Partial<NetworkOverview> = {}): NetworkOverview => ({
  posture: 'quiet',
  devices: { known: 3, unnamed: 0, new: 0, active: 3 },
  findings: { scanned: true, open: 0, serious: 0, new: 0 },
  gateway_changed: false, shared_address: false, presence: {}, series: [], recent: [], ...over,
})

describe('attentionReasons', () => {
  it('says nothing for a quiet network', () => {
    expect(attentionReasons(overview())).toEqual([])
  })

  it('lists the reasons most serious first, in plain words with correct plurals', () => {
    const o = overview({ posture: 'attention', gateway_changed: true, shared_address: true, findings: { scanned: true, open: 3, serious: 1, new: 0 }, devices: { known: 5, unnamed: 0, new: 2, active: 5 } })
    expect(attentionReasons(o)).toEqual([
      "The router's hardware address changed",
      '1 serious finding in the last audit',
      '2 new devices nobody has approved',
      'Two addresses answer as one device',
    ])
  })
})

describe('postureHeadline', () => {
  it('has one headline per posture', () => {
    expect(postureHeadline(overview({ posture: 'unknown' }))).toBe('Not looked at yet')
    expect(postureHeadline(overview({ posture: 'attention' }))).toBe('Something needs a look')
  })
})

describe('seriesOf', () => {
  const day = (date: string, devices: number): NetworkDay => ({ date, devices_up: 0, devices, unnamed: 0, findings: 0, serious: 0 })

  it('keeps the order and carries a missing figure forward so a gap is not drawn as a drop', () => {
    const days = [day('a', 4), day('b', 0), day('c', 6)]
    expect(seriesOf(days, (d) => (d.date === 'b' ? null : d.devices))).toEqual([4, 4, 6])
  })

  it('is empty for no days', () => {
    expect(seriesOf([], (d) => d.devices)).toEqual([])
  })
})

describe('changeText', () => {
  it('names a device by the name the operator gave, else its address', () => {
    expect(changeText({ kind: 'new_device', at: 'x', name: 'Printer', ip: '192.168.1.9' })).toBe('Printer appeared on the network')
    expect(changeText({ kind: 'new_device', at: 'x', ip: '192.168.1.9' })).toBe('192.168.1.9 appeared on the network')
    expect(changeText({ kind: 'address_changed', at: 'x', ip: '192.168.1.9' })).toBe('192.168.1.9 moved to a different device')
  })
})

describe('nextAction', () => {
  const base: NetworkOverview = {
    posture: 'quiet', devices: { known: 3, unnamed: 0, new: 0, active: 3 }, findings: { scanned: true, open: 0, serious: 0, new: 0 },
    gateway_changed: false, shared_address: false, presence: {}, series: [], recent: [],
  }
  const withOver = (over: Partial<NetworkOverview>): NetworkOverview => ({ ...base, ...over })

  it('offers nothing while the network is quiet', () => expect(nextAction(base, true)).toBeNull())
  it('sends a new device to the device list', () => expect(nextAction(withOver({ posture: 'attention', devices: { ...base.devices, new: 1 } }), true)).toEqual({ label: 'Review devices', tab: 'devices' }))
  it('puts a changed router before serious findings', () => {
    expect(nextAction(withOver({ posture: 'attention', gateway_changed: true, findings: { ...base.findings, serious: 2 } }), true)?.tab).toBe('devices')
  })
  it('sends serious findings to the findings', () => expect(nextAction(withOver({ posture: 'attention', findings: { ...base.findings, serious: 1 } }), true)).toEqual({ label: 'Review findings', tab: 'findings' }))
  it('still points somewhere for any other reason to look', () => expect(nextAction(withOver({ posture: 'attention', shared_address: true }), true)?.tab).toBe('devices'))
  it('shows a network nobody has looked at how to start', () => {
    expect(nextAction(withOver({ posture: 'unknown' }), false)).toEqual({ label: 'Set up a network audit', tab: 'audits' })
    expect(nextAction(withOver({ posture: 'unknown' }), true)).toEqual({ label: 'Turn on the new-device check', tab: 'settings' })
  })
})

describe('the router count on the overview', () => {
  const day = (date: string, blocked?: number): NetworkDay => ({ date, devices_up: 1, devices: 1, unnamed: 0, findings: 0, serious: 0, blocked })

  it('is shown only once the router has been counted on some day', () => {
    expect(hasRouterCounts([day('2026-11-03'), day('2026-11-04')])).toBe(false)
    expect(hasRouterCounts([day('2026-11-03', 5), day('2026-11-04')])).toBe(true)
    expect(hasRouterCounts([])).toBe(false)
  })

  it('is today\'s count, and zero when today has none yet', () => {
    const now = new Date(2026, 10, 4, 15, 0).getTime()
    expect(refusedToday([day('2026-11-03', 9), day('2026-11-04', 40)], now)).toBe(40)
    expect(refusedToday([day('2026-11-03', 9)], now)).toBe(0)
  })
})
