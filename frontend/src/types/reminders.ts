// Reminders as GET /admin/api/reminders reports them (backend reminderView).
export type ReminderStatus = 'scheduled' | 'done' | 'missed' | 'failed' | 'cancelled'

export interface ReminderItem {
  id: number
  message: string
  // RFC 3339, UTC.
  fire_at: string
  // Present for a recurring reminder.
  cron?: string
  // IANA zone the reminder was set in.
  timezone: string
  // Connector type it goes to, or "in-app".
  channel: string
  status: ReminderStatus
  last_error?: string
}

export interface ReminderList {
  open: ReminderItem[]
  recent: ReminderItem[]
}
