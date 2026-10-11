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
  // Last occurrence of a recurring reminder (RFC 3339).
  until?: string
  // The channel as chosen: "" = automatic, "in-app", or a connector type.
  connector: string
  // Where it will actually go: a connector type, or "in-app".
  channel: string
  status: ReminderStatus
  last_error?: string
}

export interface ReminderList {
  open: ReminderItem[]
  recent: ReminderItem[]
}

// The edit form, as PUT /admin/api/reminders/{id} takes it: give fire_at for a
// one-off, or cron (with an optional until) for a recurring reminder.
export interface ReminderEdit {
  message: string
  fire_at?: string
  cron?: string
  until?: string
  connector: string
}
