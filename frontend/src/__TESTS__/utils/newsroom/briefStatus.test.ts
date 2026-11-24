import { describe, expect, it } from 'vitest'
import { briefStatus, nextReady } from '../../../utils/newsroom/briefStatus'

const edition = { ready_by: '07:00', days: [0, 1, 2, 3, 4, 5, 6], enabled: true }
const at = (iso: string) => new Date(iso)

describe('briefStatus', () => {
  it('says today\'s brief was written automatically and nothing needs pressing', () => {
    const s = briefStatus(edition, { made_at: at('2026-11-24T07:07:00').toISOString(), ready_for: '2026-11-24T07:00:00Z' }, at('2026-11-24T09:00:00'))
    expect(s.tone).toBe('fresh')
    expect(s.title).toBe("Today's brief")
    expect(s.detail).toContain('written automatically')
    expect(s.detail).toContain('Nothing to press')
    expect(s.detail).toContain('tomorrow')
  })

  it('says a brief written by hand was written by hand', () => {
    expect(briefStatus(edition, { made_at: at('2026-11-24T08:00:00').toISOString() }, at('2026-11-24T09:00:00')).detail).toContain('by hand')
  })

  it('says loudly that an earlier day\'s brief is not today\'s and when the next one comes', () => {
    const s = briefStatus(edition, { made_at: at('2026-11-23T07:07:00').toISOString(), ready_for: 'x' }, at('2026-11-24T05:00:00'))
    expect(s.tone).toBe('old')
    expect(s.title).toContain('This brief is from')
    expect(s.detail).toContain("not today's")
    expect(s.detail).toContain('today at')
    expect(s.detail).toContain('do not have to press')
  })

  it('says when there is none yet, and when an edition is paused', () => {
    expect(briefStatus(edition, null, at('2026-11-24T05:00:00')).tone).toBe('none')
    expect(briefStatus({ ...edition, enabled: false }, null).detail).toContain('paused')
  })
})

describe('nextReady', () => {
  it('skips a day the edition does not run', () => {
    const weekdays = { ready_by: '07:00', days: [1, 2, 3, 4, 5] }
    expect(nextReady(weekdays, at('2026-11-27T09:00:00'))!.getDay()).toBe(1) // Friday after 07:00 -> Monday
  })
  it('gives nothing for a bad time', () => expect(nextReady({ ready_by: 'soon', days: [] }, new Date())).toBeNull())
})
