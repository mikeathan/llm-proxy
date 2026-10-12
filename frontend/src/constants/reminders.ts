import type { ReminderStatus } from '../types/reminders'
import type { ScheduleKind } from '../types/schedule'
import type { StatusState } from '../types/ui'

export const REMINDERS_POLL_MS = 30_000
export const RECENT_REMINDERS_SHOWN = 5

// Backend reminders.InAppChannel: no connector, shown here only.
export const IN_APP_CHANNEL = 'in-app'

export const RELOAD_HINT = 'Check that the service is running, then reload the page.'

export const REMINDER_EXAMPLE = 'Ask the assistant: "remind me tomorrow morning to call the bank".'

// The outcome of a finished reminder, in plain words.
export const REMINDER_OUTCOME_TAG: Partial<Record<ReminderStatus, { state: StatusState; label: string }>> = {
  done: { state: 'success', label: 'Sent' },
  missed: { state: 'warning', label: 'Missed' },
  failed: { state: 'error', label: 'Failed' },
}

// Default-channel choices on the settings page.
export const AUTOMATIC_CHANNEL_LABEL = 'Automatic'
export const DEFAULT_CHANNEL_HINT =
  'Where a reminder goes when you do not name a channel. One asked for in a chat app is answered there. Automatic uses your only connected connector; with several and no choice here, it is shown in the web UI only.'
export const TIMEZONE_HINT =
  'Used when a reminder comes from a chat app, which does not report your timezone. The web UI always uses your browser timezone.'
export const UNKNOWN_TIMEZONE_ERROR = 'Not a known timezone. Use a name such as Europe/London.'

// A reminder repeats at a time of day, never "every few minutes" or "manual only".
export const REMINDER_REPEAT_KINDS = ['daily', 'weekdays', 'weekly', 'monthly', 'custom'] as const satisfies readonly ScheduleKind[]

// A recurring reminder's end date means the end of that day.
export const REMINDER_UNTIL_TIME = '23:59'

// The channel choices that exist whatever is connected.
export const IN_APP_CHANNEL_LABEL = 'Web UI only'
