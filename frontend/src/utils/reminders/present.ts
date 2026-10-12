import { describeSchedule, fromTrigger } from '../automation/schedule'
import { formatAbsoluteTime } from '../format/time'
import type { ReminderItem } from '../../types/reminders'

// How a reminder reads on screen. One place, so the Overview panel and the
// Reminders page cannot drift apart.

/** When it fires, in the reminder's own timezone. */
export const reminderWhen = (r: ReminderItem, locale?: string): string => formatAbsoluteTime(r.fire_at, locale, r.timezone)

/** How often a repeating reminder fires in plain words; empty for a one-off. */
export const reminderRepeat = (r: ReminderItem): string => (r.cron ? describeSchedule(fromTrigger('cron', r.cron)) : '')

/** When a repeating reminder ends ("until …"); empty when it has no end. */
export const reminderEnds = (r: ReminderItem, locale?: string): string =>
  r.until ? `until ${formatAbsoluteTime(r.until, locale, r.timezone)}` : ''
