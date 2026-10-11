import { describe, it, expect } from 'vitest'
import { formatRunTime } from '../../../utils/automation/runTime'

describe('formatRunTime', () => {
  // The wall clock in the timestamp's own offset is what the schedule means,
  // whatever zone the browser is in.
  it.each([
    ['2026-10-12T07:00:00+02:00', 'Mon 12 Oct, 07:00'],
    ['2026-10-13T00:00:00Z', 'Tue 13 Oct, 00:00'],
    ['2026-12-31T23:59:00-05:00', 'Thu 31 Dec, 23:59'],
    ['2026-10-12T07:00:00.123456+02:00', 'Mon 12 Oct, 07:00'],
  ])('%s → %s', (iso, text) => {
    expect(formatRunTime(iso)).toBe(text)
  })

  it('returns the input when it is not a timestamp', () => {
    expect(formatRunTime('soon')).toBe('soon')
  })
})
