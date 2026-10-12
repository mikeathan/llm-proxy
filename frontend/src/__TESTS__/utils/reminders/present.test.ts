import { describe, it, expect } from 'vitest'
import { reminderEnds, reminderRepeat, reminderWhen } from '../../../utils/reminders/present'
import type { ReminderItem } from '../../../types/reminders'

const base: ReminderItem = {
  id: 1, message: 'x', fire_at: '2026-10-12T08:00:00Z', timezone: 'Europe/London', connector: '', channel: 'telegram', status: 'scheduled',
}

describe('reminder presentation', () => {
  it('shows the time in the reminders own timezone', () => {
    expect(reminderWhen(base, 'en-GB')).toContain('09:00') // BST
    expect(reminderWhen({ ...base, timezone: 'Asia/Tokyo' }, 'en-GB')).toContain('17:00')
  })

  it('does not throw for a zone the browser does not know', () => {
    expect(() => reminderWhen({ ...base, timezone: 'Local' }, 'en-GB')).not.toThrow()
  })

  it('says how often a repeating reminder fires, and nothing for a one-off', () => {
    expect(reminderRepeat({ ...base, cron: '0 9 * * 1' })).toBe('Every Monday at 09:00')
    expect(reminderRepeat(base)).toBe('')
  })

  it('says when a repeating reminder ends', () => {
    expect(reminderEnds({ ...base, cron: '0 9 * * 1', until: '2026-12-31T23:59:00Z' }, 'en-GB')).toMatch(/^until .*2026/)
    expect(reminderEnds({ ...base, cron: '0 9 * * 1' })).toBe('')
  })
})
