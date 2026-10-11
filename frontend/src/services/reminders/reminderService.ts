import type { ReminderEdit, ReminderItem, ReminderList } from '../../types/reminders'

// The backend answers errors as {"error": "…"}; show the reason, not the JSON.
async function failureReason(res: Response, what: string): Promise<Error> {
  const text = await res.text()
  try {
    const reason = (JSON.parse(text) as { error?: string }).error
    if (reason) return new Error(reason)
  } catch {
    // not JSON: fall through to the raw text
  }
  return new Error(`Failed to ${what}: ${res.status} - ${text}`)
}

export class ReminderService {
  static async list(): Promise<ReminderList> {
    const res = await fetch('/admin/api/reminders')
    if (!res.ok) throw await failureReason(res, 'list reminders')
    return res.json()
  }

  static async edit(id: number, edit: ReminderEdit): Promise<ReminderItem> {
    const res = await fetch(`/admin/api/reminders/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(edit),
    })
    if (!res.ok) throw await failureReason(res, 'save reminder')
    return res.json()
  }

  static async remove(id: number): Promise<void> {
    const res = await fetch(`/admin/api/reminders/${id}`, { method: 'DELETE' })
    if (!res.ok) throw await failureReason(res, 'delete reminder')
  }
}
