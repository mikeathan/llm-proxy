import type { ReminderList } from '../../types/reminders'

// The backend answers errors as {"error": "…"}; show the reason, not the JSON.
async function failureReason(res: Response): Promise<Error> {
  const text = await res.text()
  try {
    const reason = (JSON.parse(text) as { error?: string }).error
    if (reason) return new Error(reason)
  } catch {
    // not JSON: fall through to the raw text
  }
  return new Error(`Failed to list reminders: ${res.status} - ${text}`)
}

export class ReminderService {
  static async list(): Promise<ReminderList> {
    const res = await fetch('/admin/api/reminders')
    if (!res.ok) throw await failureReason(res)
    return res.json()
  }
}
