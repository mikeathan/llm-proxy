import { describe, it, expect } from 'vitest'
import { browserTimeZone, defaultReminderTime, fromZonedInput, isKnownTimeZone, toZonedInput } from '../../utils/format/timezone'

describe('timezone helpers', () => {
  it('accepts IANA names and rejects anything else', () => {
    expect(isKnownTimeZone('Europe/London')).toBe(true)
    expect(isKnownTimeZone('Asia/Tokyo')).toBe(true)
    expect(isKnownTimeZone('Mars/Base')).toBe(false)
    expect(isKnownTimeZone('tomorrow')).toBe(false)
  })

  it('treats an empty value as valid (unset)', () => {
    expect(isKnownTimeZone('')).toBe(true)
  })

  it("reads the browser's own zone", () => {
    expect(isKnownTimeZone(browserTimeZone())).toBe(true)
  })
})

describe('zoned date-time input', () => {
  it('shows a UTC instant as wall-clock time in the zone', () => {
    expect(toZonedInput('2026-10-12T08:00:00Z', 'Europe/London')).toBe('2026-10-12T09:00') // BST
    expect(toZonedInput('2026-12-12T08:00:00Z', 'Europe/London')).toBe('2026-12-12T08:00') // GMT
    expect(toZonedInput('2026-10-12T08:00:00Z', 'Asia/Tokyo')).toBe('2026-10-12T17:00')
  })

  it('reads wall-clock time in the zone back to the UTC instant', () => {
    expect(fromZonedInput('2026-10-12T09:00', 'Europe/London')).toBe('2026-10-12T08:00:00.000Z')
    expect(fromZonedInput('2026-12-12T08:00', 'Europe/London')).toBe('2026-12-12T08:00:00.000Z')
    expect(fromZonedInput('2026-10-12T17:00', 'Asia/Tokyo')).toBe('2026-10-12T08:00:00.000Z')
  })

  it('round-trips either side of a clock change', () => {
    for (const iso of ['2026-10-24T22:30:00.000Z', '2026-10-25T03:30:00.000Z', '2026-03-29T12:00:00.000Z']) {
      expect(fromZonedInput(toZonedInput(iso, 'Europe/London'), 'Europe/London')).toBe(iso)
    }
  })

  it('picks one of the two real instants for a wall time that happens twice', () => {
    // 01:30 on 25 Oct 2026 in London occurs as 00:30Z (BST) and 01:30Z (GMT).
    const got = fromZonedInput('2026-10-25T01:30', 'Europe/London')
    expect(['2026-10-25T00:30:00.000Z', '2026-10-25T01:30:00.000Z']).toContain(got)
  })

  it('treats UTC and an unknown zone as UTC', () => {
    expect(toZonedInput('2026-10-12T08:00:00Z', 'UTC')).toBe('2026-10-12T08:00')
    expect(toZonedInput('2026-10-12T08:00:00Z', 'Local')).toBe('2026-10-12T08:00')
    expect(fromZonedInput('2026-10-12T08:00', 'Local')).toBe('2026-10-12T08:00:00.000Z')
  })

  it('returns an empty string / null for an empty or invalid value', () => {
    expect(fromZonedInput('', 'UTC')).toBeNull()
    expect(fromZonedInput('not a date', 'UTC')).toBeNull()
    expect(toZonedInput('', 'UTC')).toBe('')
  })
})

describe('defaultReminderTime', () => {
  it('is 09:00 tomorrow in the zone', () => {
    const now = new Date('2026-10-12T20:30:00Z') // 21:30 BST on the 12th
    expect(defaultReminderTime(now, 'Europe/London')).toBe('2026-10-13T08:00:00.000Z') // 09:00 BST on the 13th
    expect(defaultReminderTime(now, 'Asia/Tokyo')).toBe('2026-10-14T00:00:00.000Z') // it is already the 13th in Tokyo, so 09:00 JST on the 14th
  })

  it('goes by the date in the zone, not in UTC', () => {
    const now = new Date('2026-10-12T14:30:00Z') // 23:30 on the 12th in Tokyo, 15:30 in London
    expect(defaultReminderTime(now, 'Asia/Tokyo')).toBe('2026-10-13T00:00:00.000Z')
    expect(defaultReminderTime(now, 'Europe/London')).toBe('2026-10-13T08:00:00.000Z')
  })
})
