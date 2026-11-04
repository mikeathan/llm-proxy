import { describe, expect, it } from 'vitest'
import { activityText, clockTime, dayHeading, filterRows, groupByDay } from '../../../utils/netaudit/activity'
import type { ActivityRow } from '../../../types/netaudit'

const row = (over: Partial<ActivityRow>): ActivityRow => ({ kind: 'new_device', group: 'devices', at: '2026-11-04T10:00:00Z', ...over })

describe('activityText', () => {
  it('names a new device by its name, else its address, else its hardware address', () => {
    expect(activityText(row({ name: 'Printer', ip: '192.168.1.5' }))).toBe('Printer appeared on the network')
    expect(activityText(row({ ip: '192.168.1.5' }))).toBe('192.168.1.5 appeared on the network')
    expect(activityText(row({ mac: 'aa:bb:cc:00:00:01' }))).toBe('aa:bb:cc:00:00:01 appeared on the network')
    expect(activityText(row({}))).toBe('A device appeared on the network')
  })

  it('says an address moved to a different device', () => {
    expect(activityText(row({ kind: 'address_changed', ip: '192.168.1.9' }))).toBe('192.168.1.9 moved to a different device')
  })

  it('shows an alert in its own words, and says how many times when it repeated', () => {
    expect(activityText(row({ kind: 'alert', group: 'alerts', text: 'A decoy service was touched', count: 1 }))).toBe('A decoy service was touched')
    expect(activityText(row({ kind: 'alert', group: 'alerts', text: 'A decoy service was touched', count: 3 }))).toBe('A decoy service was touched (3 times)')
  })

  it('counts ports opened and closed in plain words, leaving out the one that is zero', () => {
    expect(activityText(row({ kind: 'ports_changed', group: 'ports', opened: 2, closed: 1 }))).toBe('2 ports opened and 1 port closed')
    expect(activityText(row({ kind: 'ports_changed', group: 'ports', opened: 1, closed: 0 }))).toBe('1 port opened')
    expect(activityText(row({ kind: 'ports_changed', group: 'ports', opened: 0, closed: 4 }))).toBe('4 ports closed')
  })

  it('says what the last audit found and what went away', () => {
    expect(activityText(row({ kind: 'audit', group: 'findings', opened: 2, closed: 1 }))).toBe('The audit found 2 new problems; 1 went away')
    expect(activityText(row({ kind: 'audit', group: 'findings', opened: 1, closed: 0 }))).toBe('The audit found 1 new problem')
    expect(activityText(row({ kind: 'audit', group: 'findings', opened: 0, closed: 2 }))).toBe('The audit found nothing new; 2 went away')
  })

  it('says only the timing for a possibly related pair', () => {
    expect(activityText(row({ kind: 'related', group: 'alerts', name: 'Camera' }))).toBe('Camera appeared within an hour before a decoy service was touched')
  })

  it('never renders markup as anything but text: the words are returned as they came', () => {
    expect(activityText(row({ name: '<img src=x onerror=alert(1)>' }))).toBe('<img src=x onerror=alert(1)> appeared on the network')
  })
})

describe('filterRows', () => {
  const rows = [row({ group: 'devices' }), row({ group: 'ports', kind: 'ports_changed' }), row({ group: 'alerts', kind: 'alert' })]
  it('keeps everything for all and one group otherwise', () => {
    expect(filterRows(rows, 'all')).toHaveLength(3)
    expect(filterRows(rows, 'ports').map((r) => r.kind)).toEqual(['ports_changed'])
    expect(filterRows(rows, 'findings')).toEqual([])
  })
})

describe('groupByDay', () => {
  const now = new Date(2026, 10, 4, 15, 0, 0).getTime()
  const local = (d: number, h: number) => new Date(2026, 10, d, h, 0, 0).toISOString()

  it('groups newest-first rows under Today, Yesterday and a dated heading, keeping their order', () => {
    const groups = groupByDay([row({ at: local(4, 12) }), row({ at: local(4, 9) }), row({ at: local(3, 20) }), row({ at: local(1, 8) })], now)
    expect(groups.map((g) => [g.heading, g.rows.length])).toEqual([['Today', 2], ['Yesterday', 1], [dayHeading(local(1, 8), now), 1]])
    expect(groups[2]!.heading).not.toBe('Today')
  })

  it('has no groups for no rows', () => {
    expect(groupByDay([], now)).toEqual([])
  })
})

describe('clockTime', () => {
  it('shows the time of a row and nothing for a row that stands for a whole day', () => {
    expect(clockTime(row({ at: new Date(2026, 10, 4, 9, 5, 0).toISOString() }), 'en-GB')).toBe('09:05')
    expect(clockTime(row({ day: true }), 'en-GB')).toBe('')
  })
})
