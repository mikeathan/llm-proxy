import { describe, it, expect } from 'vitest'
import { describeSchedule, fromTrigger, toTrigger } from '../../../utils/automation/schedule'
import type { Schedule } from '../../../types/schedule'

// Every preset and its exact stored form. The table drives both directions, so
// a preset can never serialise to something it does not read back as.
const PRESETS: [string, Schedule, { type: string; value: string }][] = [
  ['manual', { kind: 'manual' }, { type: 'manual', value: '' }],
  ['every 15 minutes', { kind: 'every', n: 15, unit: 'minutes' }, { type: 'interval', value: '15m' }],
  ['every 2 hours', { kind: 'every', n: 2, unit: 'hours' }, { type: 'interval', value: '2h' }],
  ['every 90 minutes', { kind: 'every', n: 90, unit: 'minutes' }, { type: 'interval', value: '90m' }],
  ['daily', { kind: 'daily', time: '07:00' }, { type: 'cron', value: '0 7 * * *' }],
  ['daily with minutes', { kind: 'daily', time: '18:45' }, { type: 'cron', value: '45 18 * * *' }],
  ['weekdays', { kind: 'weekdays', time: '07:00' }, { type: 'cron', value: '0 7 * * 1-5' }],
  ['weekly', { kind: 'weekly', days: [1, 5], time: '09:30' }, { type: 'cron', value: '30 9 * * 1,5' }],
  ['weekly on sunday', { kind: 'weekly', days: [0], time: '00:00' }, { type: 'cron', value: '0 0 * * 0' }],
  ['monthly', { kind: 'monthly', day: 1, time: '08:00' }, { type: 'cron', value: '0 8 1 * *' }],
]

describe('toTrigger / fromTrigger', () => {
  it.each(PRESETS)('%s round-trips', (_name, schedule, trigger) => {
    expect(toTrigger(schedule)).toEqual(trigger)
    expect(fromTrigger(trigger.type as 'cron', trigger.value)).toEqual(schedule)
  })

  it('serialises weekly days sorted and without duplicates', () => {
    expect(toTrigger({ kind: 'weekly', days: [5, 1, 5], time: '09:30' }).value).toBe('30 9 * * 1,5')
  })

  it.each([
    ['cron', '*/5 * * * *'],
    ['cron', '0 7 * * 7'],
    ['cron', '0 7 * * 1-3'],
    ['cron', '@daily'],
    ['cron', '0 0 */2 * *'],
    ['cron', '0 7 1 6 *'],
    ['cron', '00 7 * * *'],
    ['cron', '0 25 * * *'],
    ['cron', 'not a cron'],
    ['interval', '30s'],
    ['interval', '1h30m15s'],
    ['interval', '0m'],
    ['interval', ''],
  ] as const)('keeps %s "%s" as custom instead of rewriting it', (type, value) => {
    expect(fromTrigger(type, value)).toEqual({ kind: 'custom', type, value })
  })

  it('stores a custom schedule untouched', () => {
    expect(toTrigger({ kind: 'custom', type: 'cron', value: '*/5 * * * *' })).toEqual({ type: 'cron', value: '*/5 * * * *' })
  })

  it('has no value until a weekly schedule names a day', () => {
    expect(toTrigger({ kind: 'weekly', days: [], time: '09:00' }).value).toBe('')
  })

  it('has no value for a count or weekday that makes no sense', () => {
    expect(toTrigger({ kind: 'every', n: 0, unit: 'minutes' }).value).toBe('')
    expect(toTrigger({ kind: 'every', n: 1.5, unit: 'hours' }).value).toBe('')
    expect(toTrigger({ kind: 'weekly', days: [7 as 0], time: '09:00' }).value).toBe('')
  })

  it('has no value for a day of the month the months do not all have', () => {
    expect(toTrigger({ kind: 'monthly', day: 31, time: '09:00' }).value).toBe('')
  })

  it('ignores a value on a manual trigger', () => {
    expect(fromTrigger('manual', 'ignored')).toEqual({ kind: 'manual' })
  })
})

// The backend stores and returns an interval as a Go Duration string, whatever was
// sent, so what comes back for a saved preset must still read as that preset.
describe('fromTrigger on the intervals the server returns', () => {
  it.each([
    ['15m0s', { kind: 'every', n: 15, unit: 'minutes' }],
    ['2h0m0s', { kind: 'every', n: 2, unit: 'hours' }],
    ['1h30m0s', { kind: 'every', n: 90, unit: 'minutes' }],
    ['1h0m0s', { kind: 'every', n: 1, unit: 'hours' }],
    ['120m', { kind: 'every', n: 2, unit: 'hours' }],
    ['1h30m', { kind: 'every', n: 90, unit: 'minutes' }],
  ] as [string, Schedule][])('%s', (value, schedule) => {
    expect(fromTrigger('interval', value)).toEqual(schedule)
  })

  it.each(['1h30m15s', '30s', '1.5h', '0s', '0h0m0s'])('keeps %s as custom', (value) => {
    expect(fromTrigger('interval', value)).toEqual({ kind: 'custom', type: 'interval', value })
  })
})

describe('describeSchedule', () => {
  it.each([
    [{ kind: 'manual' }, 'Manual only'],
    [{ kind: 'every', n: 1, unit: 'hours' }, 'Every hour'],
    [{ kind: 'every', n: 15, unit: 'minutes' }, 'Every 15 minutes'],
    [{ kind: 'every', n: 1, unit: 'minutes' }, 'Every minute'],
    [{ kind: 'every', n: 90, unit: 'minutes' }, 'Every 90 minutes'],
    [{ kind: 'daily', time: '07:00' }, 'Every day at 07:00'],
    [{ kind: 'weekdays', time: '07:00' }, 'Every weekday at 07:00'],
    [{ kind: 'weekly', days: [1], time: '09:30' }, 'Every Monday at 09:30'],
    [{ kind: 'weekly', days: [5, 1], time: '09:30' }, 'Every Monday and Friday at 09:30'],
    [{ kind: 'weekly', days: [1, 3, 5], time: '09:30' }, 'Every Monday, Wednesday and Friday at 09:30'],
    [{ kind: 'weekly', days: [], time: '09:30' }, 'Choose at least one day'],
    [{ kind: 'monthly', day: 1, time: '08:00' }, 'On the 1st of every month at 08:00'],
    [{ kind: 'monthly', day: 22, time: '08:00' }, 'On the 22nd of every month at 08:00'],
    [{ kind: 'monthly', day: 3, time: '08:00' }, 'On the 3rd of every month at 08:00'],
    [{ kind: 'custom', type: 'cron', value: '0 7 * * *' }, 'At 07:00 AM'],
    [{ kind: 'custom', type: 'cron', value: 'not a cron' }, 'Invalid schedule: not a cron'],
    // Seconds are not supported: cronstrue would read this as "Every 10 seconds".
    [{ kind: 'custom', type: 'cron', value: '*/10 * * * * *' }, 'Invalid schedule: */10 * * * * *'],
    [{ kind: 'custom', type: 'cron', value: '@daily' }, 'At 12:00 AM'],
    [{ kind: 'custom', type: 'cron', value: '' }, 'Set when it runs'],
    [{ kind: 'custom', type: 'interval', value: '30s' }, 'Every 30s'],
    [{ kind: 'custom', type: 'interval', value: '1h30m0s' }, 'Every 1h30m'],
  ] as [Schedule, string][])('%o → %s', (schedule, text) => {
    expect(describeSchedule(schedule)).toBe(text)
  })
})
