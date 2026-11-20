import type { Headline, NewsroomDay, NewsroomTopic, SourceKind, SourceRow, TopicStatus } from '../../types/newsroom'

/** The newest findings across every topic and day, newest first: each topic's current finding and the earlier ones of its day. */
export function latestHeadlines(days: readonly NewsroomDay[], limit: number): Headline[] {
  const all = days.flatMap((d) => d.topics.flatMap((t) => [t.current, ...(t.earlier ?? [])].map((entry) => ({ topic: t.topic, entry }))))
  return all.sort((a, b) => Date.parse(b.entry.at) - Date.parse(a.entry.at)).slice(0, limit)
}

/** The topics whose last check could not read a source. */
export const failingTopics = (topics: readonly NewsroomTopic[]): NewsroomTopic[] => topics.filter((t) => t.enabled && t.last_status === 'failing')

const OK: ReadonlySet<TopicStatus> = new Set<TopicStatus>(['baseline', 'nothing_new', 'in_feed', 'sent', 'send_failed', 'no_channel', 'analysis_failed'])
const sourceKey = (kind: SourceKind, target: string, instance?: string): string => [kind, instance ?? '', target.trim().toLowerCase()].join('|')

/** Every distinct source across the topics. A source is failing if any enabled topic reading it failed its last check; paused when every topic reading it is paused. */
export function sourceRows(topics: readonly NewsroomTopic[]): SourceRow[] {
  const rows = new Map<string, SourceRow>()
  for (const t of topics) {
    for (const s of t.sources) {
      const key = sourceKey(s.kind, s.target, s.instance)
      const row = rows.get(key) ?? { key, source: s, topics: [], state: 'paused' as const, detail: '' }
      row.topics.push(t)
      rows.set(key, row)
    }
  }
  for (const row of rows.values()) {
    const live = row.topics.filter((t) => t.enabled)
    const failed = live.find((t) => t.last_status === 'failing')
    if (failed) { row.state = 'failing'; row.detail = failed.last_detail ?? '' }
    else if (live.some((t) => t.last_status && OK.has(t.last_status))) row.state = 'working'
    else row.state = live.length ? 'unchecked' : 'paused'
  }
  return [...rows.values()]
}
