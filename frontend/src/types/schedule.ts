import type { TriggerType } from './automation'

// A time of day as "HH:MM" (24-hour), the value of <input type="time">.
export type TimeOfDay = string

// Days of the week as cron numbers: 0 = Sunday … 6 = Saturday.
export type Weekday = 0 | 1 | 2 | 3 | 4 | 5 | 6

export type EveryUnit = 'minutes' | 'hours'

// How a person thinks about when an automation runs. Each kind serialises to a
// backend trigger and is read back from one (utils/automation/schedule.ts);
// anything that is not an exact preset stays 'custom', never rewritten.
export type Schedule =
  | { kind: 'manual' }
  | { kind: 'every'; n: number; unit: EveryUnit }
  | { kind: 'daily'; time: TimeOfDay }
  | { kind: 'weekdays'; time: TimeOfDay }
  | { kind: 'weekly'; days: Weekday[]; time: TimeOfDay }
  | { kind: 'monthly'; day: number; time: TimeOfDay }
  | { kind: 'custom'; type: Exclude<TriggerType, 'manual'>; value: string }

export type ScheduleKind = Schedule['kind']

// The backend answer to POST /admin/api/dispatcher/schedule/preview
// (models.SchedulePreview). next_runs are RFC 3339 timestamps.
export interface SchedulePreview {
  valid: boolean
  error?: string
  timezone: string
  next_runs: string[]
}
