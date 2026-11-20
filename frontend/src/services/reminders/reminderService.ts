import type { ReminderEdit, ReminderItem, ReminderList } from '../../types/reminders'
import type { ReminderEvents } from '../../types/notifications'
import { apiFetch } from '../httpClient'
import { failureReason } from '../failureReason'

export class ReminderService {
  static async list(): Promise<ReminderList> {
    const res = await apiFetch('/admin/api/reminders')
    if (!res.ok) throw await failureReason(res, 'list reminders')
    return res.json()
  }

  /**
   * What reminders did after the given time (unix milliseconds), newest first. With no time it returns no items, only the
   * newest event time, as the place to start watching from.
   */
  static async events(after?: number): Promise<ReminderEvents> {
    const query = after === undefined ? '' : `?after=${after}`
    const res = await apiFetch(`/admin/api/reminders/events${query}`)
    if (!res.ok) throw await failureReason(res, 'read reminder events')
    return res.json()
  }

  static async create(edit: ReminderEdit): Promise<ReminderItem> {
    const res = await apiFetch('/admin/api/reminders', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(edit),
    })
    if (!res.ok) throw await failureReason(res, 'create reminder')
    return res.json()
  }

  static pause = (id: number) => ReminderService.act(id, 'pause')
  static resume = (id: number) => ReminderService.act(id, 'resume')
  static skip = (id: number) => ReminderService.act(id, 'skip')

  private static async act(id: number, action: 'pause' | 'resume' | 'skip'): Promise<ReminderItem> {
    const res = await apiFetch(`/admin/api/reminders/${id}/${action}`, { method: 'POST' })
    if (!res.ok) throw await failureReason(res, `${action} reminder`)
    return res.json()
  }

  static async edit(id: number, edit: ReminderEdit): Promise<ReminderItem> {
    const res = await apiFetch(`/admin/api/reminders/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(edit),
    })
    if (!res.ok) throw await failureReason(res, 'save reminder')
    return res.json()
  }

  static async remove(id: number): Promise<void> {
    const res = await apiFetch(`/admin/api/reminders/${id}`, { method: 'DELETE' })
    if (!res.ok) throw await failureReason(res, 'delete reminder')
  }
}
