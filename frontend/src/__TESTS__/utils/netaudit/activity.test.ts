import { describe, expect, it } from 'vitest'
import { activityText, clockTime, dayHeading, daySummary, dropEchoedAlerts, filterRows, foldRows, groupByDay, portLabel, portsLine, rowLink } from '../../../utils/netaudit/activity'
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

describe('daySummary', () => {
  const day = (rows: Partial<ActivityRow>[]): ActivityRow[] => rows.map((r) => row(r))
  it('counts what happened that day in one short line, in a fixed order', () => {
    expect(daySummary(day([
      { kind: 'alert', group: 'alerts', count: 2, text: 'x' },
      { kind: 'new_device' }, { kind: 'new_device' }, { kind: 'new_device' }, { kind: 'new_device' },
      { kind: 'ports_changed', group: 'ports', opened: 1, closed: 1, day: true },
    ]))).toBe('4 new devices · 1 port opened and 1 port closed · 2 alerts')
  })

  it('uses the singular, and names an address that moved, an audit and a possibly related pair', () => {
    expect(daySummary(day([{ kind: 'new_device' }]))).toBe('1 new device')
    expect(daySummary(day([{ kind: 'address_changed' }, { kind: 'audit', group: 'findings', opened: 1, closed: 0 }, { kind: 'related', group: 'alerts' }]))).toBe('1 moved address · audit result · possibly related')
  })

  it('says nothing for no rows', () => {
    expect(daySummary([])).toBe('')
  })
})

describe('foldRows', () => {
  const devices = (n: number): ActivityRow[] => Array.from({ length: n }, (_, i) => row({ name: `Dev${i}`, at: `2026-11-04T1${i}:00:00Z` }))

  it('keeps one or two new devices as they are', () => {
    expect(foldRows(devices(2)).map((i) => i.kind)).toEqual(['row', 'row'])
  })

  it('folds three or more new devices of a day into one row that names the first three', () => {
    const items = foldRows([...devices(5), row({ kind: 'ports_changed', group: 'ports', opened: 1, day: true })])
    expect(items.map((i) => i.kind)).toEqual(['fold', 'row'])
    const fold = items[0]!
    expect(fold.kind === 'fold' && fold.rows).toHaveLength(5)
    expect(fold.kind === 'fold' && fold.text).toBe('5 new devices appeared: Dev0, Dev1, Dev2 and 2 more')
  })

  it('puts the fold where the newest of the folded rows was', () => {
    const rows = [row({ kind: 'alert', group: 'alerts', text: 'a' }), ...devices(3), row({ kind: 'audit', group: 'findings', opened: 1 })]
    expect(foldRows(rows).map((i) => i.kind)).toEqual(['row', 'fold', 'row'])
  })

  it('never folds a possibly related row, which has its own meaning', () => {
    const rows = [row({ kind: 'related', group: 'alerts' }), ...devices(3)]
    expect(foldRows(rows).filter((i) => i.kind === 'row').map((i) => i.kind === 'row' && i.row.kind)).toEqual(['related'])
  })
})

describe('dropEchoedAlerts', () => {
  const at = (m: number) => new Date(Date.UTC(2026, 10, 4, 10, m)).toISOString()
  it('drops a presence alert that follows a new device by under an hour, and keeps every other alert', () => {
    const rows = [
      row({ kind: 'alert', group: 'alerts', alert_kind: 'presence', text: 'Network audit: 1 new device', at: at(20) }),
      row({ kind: 'alert', group: 'alerts', alert_kind: 'decoy', text: 'decoy', at: at(25) }),
      row({ kind: 'new_device', at: at(10) }),
    ]
    expect(dropEchoedAlerts(rows).map((r) => r.alert_kind ?? r.kind)).toEqual(['decoy', 'new_device'])
  })

  it('keeps the scan-on-arrival result that follows the new device it is about', () => {
    const rows = [
      row({ kind: 'alert', group: 'alerts', alert_kind: 'follow_up', text: 'scanned on arrival', at: at(15) }),
      row({ kind: 'new_device', at: at(10) }),
    ]
    expect(dropEchoedAlerts(rows)).toHaveLength(2)
  })

  it('keeps a presence alert with no new device before it, or one an hour or more later', () => {
    const lone = row({ kind: 'alert', group: 'alerts', alert_kind: 'presence', at: at(5) })
    expect(dropEchoedAlerts([lone])).toHaveLength(1)
    const late = row({ kind: 'alert', group: 'alerts', alert_kind: 'presence', at: at(90) })
    expect(dropEchoedAlerts([late, row({ kind: 'new_device', at: at(0) })])).toHaveLength(2)
  })

  it('keeps a presence alert that came before the device, which it cannot be about', () => {
    const early = row({ kind: 'alert', group: 'alerts', alert_kind: 'presence', at: at(0) })
    expect(dropEchoedAlerts([early, row({ kind: 'new_device', at: at(10) })])).toHaveLength(2)
  })
})

describe('router blocks', () => {
  const blocks = (over: Partial<ActivityRow> = {}): ActivityRow => row({ kind: 'router_blocks', group: 'router', day: true, count: 1204, ports: [{ port: 22, count: 700 }, { port: 3306, count: 300 }, { port: 8123, count: 100 }, { port: 23, count: 50 }], ...over })

  it('names a port by what it is when that is well known, and by its number when it is not', () => {
    expect(portLabel(22)).toBe('SSH (22)')
    expect(portLabel(3306)).toBe('MySQL (3306)')
    expect(portLabel(8123)).toBe('port 8123')
  })

  it('says how many attempts the router refused and the three most tried ports', () => {
    expect(activityText(blocks())).toBe('The router refused 1,204 connection attempts from the internet, mostly to SSH (22), MySQL (3306) and port 8123')
  })

  it('leaves out the ports when there are none, and uses the singular', () => {
    expect(activityText(blocks({ ports: undefined, count: 1 }))).toBe('The router refused 1 connection attempt from the internet')
  })

  it('counts a day of refusals in the day summary', () => {
    expect(daySummary([blocks()])).toBe('1,204 refused at the router')
  })

  it('lists the busiest ports with their counts on one line', () => {
    expect(portsLine([{ port: 22, count: 700 }, { port: 3306, count: 1300 }, { port: 8123, count: 5 }, { port: 23, count: 1 }])).toBe('SSH (22) 700 · MySQL (3306) 1,300 · port 8123 5')
    expect(portsLine(undefined)).toBe('')
  })

  it('is its own filter group', () => {
    expect(filterRows([blocks(), row({})], 'router')).toHaveLength(1)
  })
})

describe('finding rows', () => {
  const finding = (over: Partial<ActivityRow>): ActivityRow => row({ group: 'findings', device: '192.0.2.10', port: 445, check: 'SMBv1 is on', ...over })

  it('says a rechecked fix and a return in plain words, from typed fields', () => {
    expect(activityText(finding({ kind: 'finding_fixed' }))).toBe('SMBv1 is on on 192.0.2.10, port 445 was checked again and is fixed')
    expect(activityText(finding({ kind: 'finding_returned', count: 2 }))).toBe('SMBv1 is on on 192.0.2.10, port 445 came back after it had gone away')
  })
})

describe('rowLink', () => {
  it('leads a fixed or returned problem to the problems of its device and an audit tally to the Findings tab', () => {
    const base = { at: '2026-11-10T10:00:00Z', group: 'findings' as const }
    expect(rowLink({ ...base, kind: 'finding_fixed', device: '192.168.50.7', check: 'X' })).toEqual({ name: 'network', params: { tab: 'findings' }, query: { device: '192.168.50.7' } })
    expect(rowLink({ ...base, kind: 'finding_returned', device: '192.168.50.7' })).toMatchObject({ query: { device: '192.168.50.7' } })
    expect(rowLink({ ...base, kind: 'audit' })).toMatchObject({ params: { tab: 'findings' } })
  })

  it('leads nowhere for a row that is not about a problem, or a problem with no device', () => {
    const base = { at: '2026-11-10T10:00:00Z', group: 'devices' as const }
    expect(rowLink({ ...base, kind: 'new_device' })).toBeNull()
    expect(rowLink({ ...base, kind: 'finding_fixed' })).toBeNull()
  })
})
