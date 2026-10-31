import { describe, expect, it } from 'vitest'
import { attentionReasons, changeText, postureHeadline, seriesOf } from '../../../utils/netaudit/networkOverview'
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
