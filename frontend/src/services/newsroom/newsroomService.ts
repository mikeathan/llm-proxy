import type { NewsroomDay, NewsroomFeedFilter, NewsroomRunOutcome, NewsroomTopic, NewsroomTopicEdit } from '../../types/newsroom'
import { apiFetch } from '../httpClient'
import { failureReason } from '../failureReason'

const BASE = '/admin/api/newsroom'
const JSON_HEADERS = { 'Content-Type': 'application/json' }

export class NewsroomService {
  static async topics(): Promise<NewsroomTopic[]> {
    const res = await apiFetch(`${BASE}/topics`)
    if (!res.ok) throw await failureReason(res, 'list topics')
    return (await res.json()) ?? []
  }

  /** The feed in days. `zone` is the zone the days are drawn in (the browser's), so Today is the reader's today. */
  static async feed(filter: NewsroomFeedFilter, zone: string): Promise<NewsroomDay[]> {
    const query = new URLSearchParams({ zone })
    if (filter.topic !== null) query.set('topic', String(filter.topic))
    if (filter.unreadOnly) query.set('unread', 'true')
    const res = await apiFetch(`${BASE}/feed?${query}`)
    if (!res.ok) throw await failureReason(res, 'read the feed')
    return (await res.json()) ?? []
  }

  static async create(edit: NewsroomTopicEdit): Promise<NewsroomTopic> {
    const res = await apiFetch(`${BASE}/topics`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(edit) })
    if (!res.ok) throw await failureReason(res, 'add the topic')
    return res.json()
  }

  static async save(id: number, edit: NewsroomTopicEdit): Promise<NewsroomTopic> {
    const res = await apiFetch(`${BASE}/topics/${id}`, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify(edit) })
    if (!res.ok) throw await failureReason(res, 'save the topic')
    return res.json()
  }

  static async remove(id: number): Promise<void> {
    const res = await apiFetch(`${BASE}/topics/${id}`, { method: 'DELETE' })
    if (!res.ok) throw await failureReason(res, 'delete the topic')
  }

  static async run(id: number): Promise<NewsroomRunOutcome> {
    const res = await apiFetch(`${BASE}/topics/${id}/run`, { method: 'POST' })
    if (!res.ok) throw await failureReason(res, 'check the topic')
    return res.json()
  }

  static async markRead(ids: number[], read: boolean): Promise<void> {
    const res = await apiFetch(`${BASE}/feed/read`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ ids, read }) })
    if (!res.ok) throw await failureReason(res, 'mark entries read')
  }
}
