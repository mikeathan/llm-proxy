import type { BriefEdition, BriefEditionEdit, BriefIssue, BriefIssueSummary, BriefJob, BriefLatest, BriefSettings, BriefSource, BriefTopic, BriefTopicEdit } from '../../types/newsroom'
import { apiFetch } from '../httpClient'
import { failureReason } from '../failureReason'

const BASE = '/admin/api/newsroom/briefs'
const JSON_HEADERS = { 'Content-Type': 'application/json' }

export class BriefService {
  static async editions(): Promise<BriefEdition[]> {
    const res = await apiFetch(`${BASE}/editions`)
    if (!res.ok) throw await failureReason(res, 'list the editions')
    return (await res.json()) ?? []
  }

  static async topics(): Promise<BriefTopic[]> {
    const res = await apiFetch(`${BASE}/topics`)
    if (!res.ok) throw await failureReason(res, 'list the topics')
    return (await res.json()) ?? []
  }

  /** The editions already made, newest first, as a list (no stories); one edition's when its id is given. */
  static async issues(editionId?: number): Promise<BriefIssueSummary[]> {
    const query = editionId === undefined ? '' : `?edition=${editionId}`
    const res = await apiFetch(`${BASE}/issues${query}`)
    if (!res.ok) throw await failureReason(res, 'read the editions')
    return (await res.json()) ?? []
  }

  /** The newest edition of each edition and whether it was read; light enough to ask every half minute. */
  static async latest(): Promise<BriefLatest[]> {
    const res = await apiFetch(`${BASE}/latest`)
    if (!res.ok) throw await failureReason(res, 'read the latest editions')
    const body: unknown = await res.json()
    return Array.isArray(body) ? (body as BriefLatest[]) : []
  }

  /** One edition as it was made. */
  static async issue(id: number): Promise<BriefIssue> {
    const res = await apiFetch(`${BASE}/issues/${id}`)
    if (!res.ok) throw await failureReason(res, 'read the edition')
    return res.json()
  }

  /** Notes that the owner read the edition; reading it again changes nothing. */
  static async markRead(id: number): Promise<void> {
    const res = await apiFetch(`${BASE}/issues/${id}/read`, { method: 'POST' })
    if (!res.ok) throw await failureReason(res, 'mark the edition read')
  }

  /** Starts writing the edition now; the write goes on in the background and is followed with jobs(). */
  static async makeNow(editionId: number): Promise<BriefJob> {
    const res = await apiFetch(`${BASE}/editions/${editionId}/issue`, { method: 'POST' })
    if (!res.ok) throw await failureReason(res, 'write the edition')
    return res.json()
  }

  /** The writes by hand that are going on, and those that ended in the last few minutes. */
  static async jobs(): Promise<BriefJob[]> {
    const res = await apiFetch(`${BASE}/jobs`)
    if (!res.ok) throw await failureReason(res, 'read how the writing is going')
    const body: unknown = await res.json()
    return Array.isArray(body) ? (body as BriefJob[]) : []
  }

  static async settings(): Promise<BriefSettings> {
    const res = await apiFetch(`${BASE}/settings`)
    if (!res.ok) throw await failureReason(res, 'read the settings')
    return res.json()
  }

  /** Sets the daily limit of model calls; zero means the default. */
  static async saveSettings(modelCallsPerDay: number): Promise<BriefSettings> {
    const res = await apiFetch(`${BASE}/settings`, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify({ model_calls_per_day: modelCallsPerDay }) })
    if (!res.ok) throw await failureReason(res, 'save the settings')
    return res.json()
  }

  /** The sources the topic form offers: the built-in list, then the owner's own. */
  static async sources(): Promise<BriefSource[]> {
    const res = await apiFetch(`${BASE}/sources`)
    if (!res.ok) throw await failureReason(res, 'list the sources')
    return (await res.json()) ?? []
  }

  static async addSource(label: string, target: string): Promise<BriefSource> {
    const res = await apiFetch(`${BASE}/sources`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ label, kind: 'feed', target }) })
    if (!res.ok) throw await failureReason(res, 'add the source')
    return res.json()
  }

  static async removeSource(id: number): Promise<void> {
    const res = await apiFetch(`${BASE}/sources/${id}`, { method: 'DELETE' })
    if (!res.ok) throw await failureReason(res, 'remove the source')
  }

  /** Forgets every topic, edition, collected story and the owner's own sources. */
  static async deleteAll(): Promise<void> {
    const res = await apiFetch(`${BASE}/all`, { method: 'DELETE' })
    if (!res.ok) throw await failureReason(res, 'delete the briefs')
  }

  static async createTopic(edit: BriefTopicEdit): Promise<BriefTopic> {
    const res = await apiFetch(`${BASE}/topics`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(edit) })
    if (!res.ok) throw await failureReason(res, 'add the topic')
    return res.json()
  }

  static async saveTopic(id: number, edit: BriefTopicEdit): Promise<BriefTopic> {
    const res = await apiFetch(`${BASE}/topics/${id}`, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify(edit) })
    if (!res.ok) throw await failureReason(res, 'save the topic')
    return res.json()
  }

  static async removeTopic(id: number): Promise<void> {
    const res = await apiFetch(`${BASE}/topics/${id}`, { method: 'DELETE' })
    if (!res.ok) throw await failureReason(res, 'delete the topic')
  }

  /** Reads the topic's sources now and says how many new stories it found. */
  static async collect(id: number): Promise<{ new: number; problems?: string[] }> {
    const res = await apiFetch(`${BASE}/topics/${id}/collect`, { method: 'POST' })
    if (!res.ok) throw await failureReason(res, 'read the topic')
    return res.json()
  }

  static async createEdition(edit: BriefEditionEdit): Promise<BriefEdition> {
    const res = await apiFetch(`${BASE}/editions`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(edit) })
    if (!res.ok) throw await failureReason(res, 'add the edition')
    return res.json()
  }

  static async saveEdition(id: number, edit: BriefEditionEdit): Promise<BriefEdition> {
    const res = await apiFetch(`${BASE}/editions/${id}`, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify(edit) })
    if (!res.ok) throw await failureReason(res, 'save the edition')
    return res.json()
  }

  static async removeEdition(id: number): Promise<void> {
    const res = await apiFetch(`${BASE}/editions/${id}`, { method: 'DELETE' })
    if (!res.ok) throw await failureReason(res, 'delete the edition')
  }
}
