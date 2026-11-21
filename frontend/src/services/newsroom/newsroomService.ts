import type { NewsroomDay, NewsroomFeedFilter, NewsroomRunOutcome, NewsroomWatch, NewsroomWatchEdit } from '../../types/newsroom'
import { apiFetch } from '../httpClient'
import { failureReason } from '../failureReason'

const BASE = '/admin/api/newsroom'
const JSON_HEADERS = { 'Content-Type': 'application/json' }

export class NewsroomService {
  static async watches(): Promise<NewsroomWatch[]> {
    const res = await apiFetch(`${BASE}/watches`)
    if (!res.ok) throw await failureReason(res, 'list watches')
    return (await res.json()) ?? []
  }

  /** The feed in days. `zone` is the zone the days are drawn in (the browser's), so Today is the reader's today. */
  static async feed(filter: NewsroomFeedFilter, zone: string): Promise<NewsroomDay[]> {
    const query = new URLSearchParams({ zone })
    if (filter.watch !== null) query.set('watch', String(filter.watch))
    if (filter.unreadOnly) query.set('unread', 'true')
    const res = await apiFetch(`${BASE}/feed?${query}`)
    if (!res.ok) throw await failureReason(res, 'read the feed')
    return (await res.json()) ?? []
  }

  static async create(edit: NewsroomWatchEdit): Promise<NewsroomWatch> {
    const res = await apiFetch(`${BASE}/watches`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(edit) })
    if (!res.ok) throw await failureReason(res, 'add the watch')
    return res.json()
  }

  static async save(id: number, edit: NewsroomWatchEdit): Promise<NewsroomWatch> {
    const res = await apiFetch(`${BASE}/watches/${id}`, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify(edit) })
    if (!res.ok) throw await failureReason(res, 'save the watch')
    return res.json()
  }

  static async remove(id: number): Promise<void> {
    const res = await apiFetch(`${BASE}/watches/${id}`, { method: 'DELETE' })
    if (!res.ok) throw await failureReason(res, 'delete the watch')
  }

  static async run(id: number): Promise<NewsroomRunOutcome> {
    const res = await apiFetch(`${BASE}/watches/${id}/run`, { method: 'POST' })
    if (!res.ok) throw await failureReason(res, 'check the watch')
    return res.json()
  }

  /** Deletes feed entries for good: the listed ids, or every read entry, or every entry. */
  static async clear(what: { ids: number[] } | { scope: 'read' | 'all' }): Promise<number> {
    const res = await apiFetch(`${BASE}/feed/clear`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(what) })
    if (!res.ok) throw await failureReason(res, 'clear the feed')
    return ((await res.json()) as { cleared: number }).cleared
  }

  static async markRead(ids: number[], read: boolean): Promise<void> {
    const res = await apiFetch(`${BASE}/feed/read`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ ids, read }) })
    if (!res.ok) throw await failureReason(res, 'mark entries read')
  }
}
