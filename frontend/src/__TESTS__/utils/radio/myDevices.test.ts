import { describe, expect, it } from 'vitest'
import { duplicateNames, hoursLine, myDeviceRows } from '../../../utils/radio/myDevices'
import type { RadioLiveDevice, RadioOwnerDevice, RadioPresenceDevice } from '../../../types/radio'

const owner = (id: string, label: string, over: Partial<RadioOwnerDevice> = {}): RadioOwnerDevice => ({
  id, label, visits: 3, first_seen: '2026-11-14', last_seen: '2026-11-18', class: 'personal', class_by: 'rules', ...over,
})
type Hour = RadioPresenceDevice['hours'][number]
const hours = (states: Hour['state'][]): Hour[] => states.map((state, i) => ({ hour: new Date(Date.UTC(2026, 10, 18, i)).toISOString(), state }))
const presence = (id: string, state: RadioPresenceDevice['state'], h: Hour[], last?: string): RadioPresenceDevice => ({
  id, label: '', state, joined_own: true, hours: h, ...(last ? { last_heard: last } : {}),
})
const live = (id: string, over: Partial<RadioLiveDevice> = {}): RadioLiveDevice => ({
  id, mine: true, class: 'personal', dbm: -60, near_m: 2, far_m: 4, ring: 'room', angle: 0, trend: 'steady', channel: 60, heard: true, tail_m: [], ...over,
} as RadioLiveDevice)

const A = 'aaaaaaaa'.padEnd(64, '0')
const B = 'bbbbbbbb'.padEnd(64, '0')
const C = 'cccccccc'.padEnd(64, '0')

describe('myDeviceRows', () => {
  it('puts the devices here now first, joins the router and motion from the live picture by short id, and keeps the rest without it', () => {
    const rows = myDeviceRows(
      [owner(A, 'Laptop'), owner(B, 'Phone'), owner(C, 'Watch')],
      [presence(A, 'away', hours(['here', 'away']), '2026-11-18T00:00:00Z'), presence(B, 'here', hours(['here', 'here']), '2026-11-18T01:00:00Z')],
      [live('bbbbbbbb', { ring: 'house', moving: true, router: { box: 'x', box_name: 'ZenWiFi', band: '5G', dbm: -70, at: 1 }, ident: { kind: 'phone', kind_confidence: 90, kind_source: 'model_name', confidence: 90 } })],
    )
    expect(rows.map((r) => r.label)).toEqual(['Phone', 'Laptop', 'Watch'])
    const phone = rows[0]!
    expect(phone.short).toBe('bbbbbbbb')
    expect(phone.state).toBe('here')
    expect(phone.moving).toBe(true)
    expect(phone.where).toBe('In the house, joined to ZenWiFi')
    expect(phone.kind).toBe('phone')
    expect(rows[1]!.where).toBe('')
    expect(rows[2]!.state).toBe('not_listening')
    expect(rows[2]!.kind).toBe('unknown')
  })

  it('counts the last 24 hours only, and the hours it listened in', () => {
    const states: Hour['state'][] = [...Array(24).fill('here'), ...Array(20).fill('away'), ...Array(4).fill('not_listening')]
    const [row] = myDeviceRows([owner(A, 'Laptop')], [presence(A, 'away', hours(states))], [])
    expect(row!.heard24).toBe(0)
    expect(row!.listened24).toBe(20)
    expect(hoursLine(row!)).toBe('Not heard in the 20 hours listened today')
  })
})

describe('hoursLine', () => {
  it('says how many of the hours listened it was heard in, and says so when nothing was listened to', () => {
    const base = myDeviceRows([owner(A, 'x')], [], [])[0]!
    expect(hoursLine({ ...base, heard24: 22, listened24: 24 })).toBe('Heard in 22 of the last 24 hours')
    expect(hoursLine({ ...base, heard24: 3, listened24: 5 })).toBe('Heard in 3 of the 5 hours listened today')
    expect(hoursLine({ ...base, heard24: 0, listened24: 0 })).toBe('Not listened to in the last 24 hours')
  })
})

describe('duplicateNames', () => {
  it('lists each name more than one device shares, most shared first', () => {
    const rows = myDeviceRows([owner(A, 'Phone or tablet'), owner(B, 'Phone or tablet'), owner(C, 'Watch')], [], [])
    expect(duplicateNames(rows)).toEqual([{ label: 'Phone or tablet', count: 2 }])
    expect(duplicateNames(rows.slice(2))).toEqual([])
  })
})
