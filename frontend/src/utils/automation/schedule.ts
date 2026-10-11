import cronstrue from 'cronstrue'
import type { TriggerType } from '../../types/automation'
import type { EveryUnit, Schedule, TimeOfDay, Weekday } from '../../types/schedule'

// The schedule a person picks ↔ the trigger the backend stores. A preset is
// recognised only when its trigger is exactly what the preset would write, so
// anything else (a hand-written cron, a 30s interval) stays 'custom' and is
// never silently rewritten. Seconds-level schedules are not supported.

const TIME_PATTERN = /^([01]\d|2[0-3]):([0-5]\d)$/
// What `shortDuration` leaves of a Go duration made of hours and minutes only.
const DURATION_PATTERN = /^(?:(\d+)h)?(?:(\d+)m)?$/
const MINUTES_PER_HOUR = 60
// minute hour day-of-month * day-of-week — the only cron shape the presets write.
const PRESET_CRON_PATTERN = /^(\d{1,2}) (\d{1,2}) (\*|\d{1,2}) \* (\*|1-5|\d(?:,\d)*)$/

const WEEKDAYS_FIELD = '1-5'
const CRON_FIELDS = 5
const ANY = '*'
// Months differ in length; capping the day keeps "monthly" from skipping short ones.
export const MAX_MONTH_DAY = 28
const WEEKDAY_NAMES = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const UNIT_SUFFIX: Record<EveryUnit, string> = { minutes: 'm', hours: 'h' }
const UNIT_NOUN: Record<EveryUnit, string> = { minutes: 'minute', hours: 'hour' }

export const NOT_SET_LABEL = 'Set when it runs'
export const NO_DAY_LABEL = 'Choose at least one day'

/** A Go duration without the zero units it prints ("2h0m0s" → "2h"). */
export function shortDuration(value: string): string {
  return value.replace(/(\D)0m/, '$1').replace(/(\D)0s$/, '$1')
}

function parseTime(time: TimeOfDay): { hour: number; minute: number } | null {
  const match = TIME_PATTERN.exec(time)
  return match ? { hour: Number(match[1]), minute: Number(match[2]) } : null
}

function cronAt(time: TimeOfDay, dayOfMonth: string, dayOfWeek: string): string {
  const parsed = parseTime(time)
  return parsed ? `${parsed.minute} ${parsed.hour} ${dayOfMonth} * ${dayOfWeek}` : ''
}

const isWeekday = (day: number): boolean => Number.isInteger(day) && day >= 0 && day < WEEKDAY_NAMES.length

/** "1,5" for a set of days; empty when there are none or any is not a day of the week. */
function weeklyField(days: Weekday[]): string {
  if (!days.every(isWeekday)) return ''
  return [...new Set(days)].sort((a, b) => a - b).join(',')
}

/** The backend trigger for a schedule. An incomplete schedule has an empty value. */
export function toTrigger(schedule: Schedule): { type: TriggerType; value: string } {
  switch (schedule.kind) {
    case 'manual':
      return { type: 'manual', value: '' }
    case 'every': {
      const valid = Number.isInteger(schedule.n) && schedule.n >= 1
      return { type: 'interval', value: valid ? `${schedule.n}${UNIT_SUFFIX[schedule.unit]}` : '' }
    }
    case 'daily':
      return { type: 'cron', value: cronAt(schedule.time, ANY, ANY) }
    case 'weekdays':
      return { type: 'cron', value: cronAt(schedule.time, ANY, WEEKDAYS_FIELD) }
    case 'weekly': {
      const field = weeklyField(schedule.days)
      return { type: 'cron', value: field ? cronAt(schedule.time, ANY, field) : '' }
    }
    case 'monthly': {
      const valid = Number.isInteger(schedule.day) && schedule.day >= 1 && schedule.day <= MAX_MONTH_DAY
      return { type: 'cron', value: valid ? cronAt(schedule.time, String(schedule.day), ANY) : '' }
    }
    case 'custom':
      return { type: schedule.type, value: schedule.value }
  }
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

function presetFromCron(value: string): Schedule | null {
  const match = PRESET_CRON_PATTERN.exec(value)
  if (!match) return null
  const [, minute = '', hour = '', dayOfMonth = '', dayOfWeek = ''] = match
  const time = `${pad(Number(hour))}:${pad(Number(minute))}`
  if (dayOfMonth === ANY) {
    if (dayOfWeek === ANY) return { kind: 'daily', time }
    if (dayOfWeek === WEEKDAYS_FIELD) return { kind: 'weekdays', time }
    return { kind: 'weekly', days: dayOfWeek.split(',').map(Number) as Weekday[], time }
  }
  return dayOfWeek === ANY ? { kind: 'monthly', day: Number(dayOfMonth), time } : null
}

// The backend returns an interval as a Go duration ("1h30m0s") whatever was sent, so
// any whole number of minutes reads as the preset that would write it: hours when it
// divides evenly, minutes otherwise.
function presetFromInterval(value: string): Schedule | null {
  const match = DURATION_PATTERN.exec(shortDuration(value))
  if (!match || (match[1] === undefined && match[2] === undefined)) return null
  const total = Number(match[1] ?? 0) * MINUTES_PER_HOUR + Number(match[2] ?? 0)
  if (total < 1) return null
  return total % MINUTES_PER_HOUR === 0
    ? { kind: 'every', n: total / MINUTES_PER_HOUR, unit: 'hours' }
    : { kind: 'every', n: total, unit: 'minutes' }
}

/** The schedule a stored trigger reads as: a preset only when it round-trips exactly. */
export function fromTrigger(type: TriggerType, value: string): Schedule {
  if (type === 'manual') return { kind: 'manual' }
  if (type === 'interval') return presetFromInterval(value) ?? { kind: 'custom', type, value }
  const preset = presetFromCron(value)
  return preset && toTrigger(preset).value === value ? preset : { kind: 'custom', type, value }
}

function ordinal(n: number): string {
  const tens = n % 100
  if (tens >= 11 && tens <= 13) return `${n}th`
  const suffix = { 1: 'st', 2: 'nd', 3: 'rd' }[n % 10] ?? 'th'
  return `${n}${suffix}`
}

function joinNames(names: string[]): string {
  if (names.length < 2) return names.join('')
  return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`
}

function describeCustom(type: Exclude<TriggerType, 'manual'>, value: string): string {
  if (!value) return NOT_SET_LABEL
  if (type === 'interval') return `Every ${shortDuration(value)}`
  // cronstrue reads a sixth (seconds) field; the scheduler does not, so do not describe one.
  if (!value.startsWith('@') && value.trim().split(/\s+/).length !== CRON_FIELDS) return `Invalid schedule: ${value}`
  try {
    return cronstrue.toString(value)
  } catch {
    return `Invalid schedule: ${value}`
  }
}

/** A plain-English reading of a schedule, for the form, the review and the list. */
export function describeSchedule(schedule: Schedule): string {
  switch (schedule.kind) {
    case 'manual':
      return 'Manual only'
    case 'every':
      return schedule.n === 1 ? `Every ${UNIT_NOUN[schedule.unit]}` : `Every ${schedule.n} ${UNIT_NOUN[schedule.unit]}s`
    case 'daily':
      return `Every day at ${schedule.time}`
    case 'weekdays':
      return `Every weekday at ${schedule.time}`
    case 'weekly': {
      if (schedule.days.length === 0) return NO_DAY_LABEL
      const names = [...new Set(schedule.days)].sort((a, b) => a - b).map((d) => WEEKDAY_NAMES[d] ?? '')
      return `Every ${joinNames(names)} at ${schedule.time}`
    }
    case 'monthly':
      return `On the ${ordinal(schedule.day)} of every month at ${schedule.time}`
    case 'custom':
      return describeCustom(schedule.type, schedule.value)
  }
}
