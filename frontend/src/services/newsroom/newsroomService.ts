import type { AlertCounts, NewsroomDay, NewsroomFeedFilter, NewsroomRunOutcome, NewsroomSource, NewsroomWatch, NewsroomWatchEdit, SourcePreview, SourceSuggestion } from '../../types/newsroom'
import { apiFetch } from '../httpClient'
import { failureReason } from '../failureReason'

const BASE = '/admin/api/newsroom'
const JSON_HEADERS = { 'Content-Type': 'application/json' }

const sourceQuery = (s: NewsroomSource): string => new URLSearchParams({ kind: s.kind, target: s.target, instance: s.instance ?? '' }).toString()

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
    if (filter.days !== null) query.set('days', String(filter.days))
    if (filter.leftOut) query.set('left_out', 'true')
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

  /** Reads one source once, without saving anything, and says what it found. */
  static async lookup(source: NewsroomSource, signal?: AbortSignal): Promise<SourcePreview> {
    const res = await apiFetch(`${BASE}/sources/lookup?${sourceQuery(source)}`, { signal })
    if (!res.ok) throw await failureReason(res, 'look the source up')
    return res.json()
  }

  /** What the owner can pick while typing (a Mastodon server's trending or matching tags). */
  static async suggest(source: NewsroomSource, signal?: AbortSignal): Promise<SourceSuggestion> {
    const res = await apiFetch(`${BASE}/sources/suggest?${sourceQuery(source)}`, { signal })
    if (!res.ok) throw await failureReason(res, 'list suggestions')
    return res.json()
  }

  /** Deletes feed entries for good: the listed ids, or every read entry, or every entry. */
  static async clear(what: { ids: number[] } | { scope: 'read' | 'all' }): Promise<number> {
    const res = await apiFetch(`${BASE}/feed/clear`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(what) })
    if (!res.ok) throw await failureReason(res, 'clear the feed')
    return ((await res.json()) as { cleared: number }).cleared
  }

  /** How many alerts wait unread, and how many items the model left out are kept. A value the page cannot read counts as none. */
  static async alertCounts(): Promise<AlertCounts> {
    const res = await apiFetch(`${BASE}/feed/unread`)
    if (!res.ok) throw await failureReason(res, 'count the unread alerts')
    const body = ((await res.json()) ?? {}) as { count?: unknown; left_out?: unknown }
    const num = (v: unknown): number => (typeof v === 'number' ? v : 0)
    return { unread: num(body.count), leftOut: num(body.left_out) }
  }

  /** Puts items the model left out into the alerts: the owner says the call was wrong. */
  static async keep(ids: number[]): Promise<void> {
    const res = await apiFetch(`${BASE}/feed/keep`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ ids }) })
    if (!res.ok) throw await failureReason(res, 'put the item in Alerts')
  }

  static async markRead(ids: number[], read: boolean): Promise<void> {
    const res = await apiFetch(`${BASE}/feed/read`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ ids, read }) })
    if (!res.ok) throw await failureReason(res, 'mark entries read')
  }
}
