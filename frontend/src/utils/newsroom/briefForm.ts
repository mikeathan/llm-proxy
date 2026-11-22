import type { BriefEdition, BriefEditionEdit, BriefTopic, BriefTopicEdit } from '../../types/newsroom'

/** One value per line, trimmed, blanks and repeats dropped. */
export const fromLines = (text: string): string[] => [...new Set(text.split('\n').map((l) => l.trim()).filter((l) => l !== ''))]

/** One value per comma, trimmed, lower case, blanks and repeats dropped. */
export const fromWords = (text: string): string[] => [...new Set(text.split(',').map((w) => w.trim().toLowerCase()).filter((w) => w !== ''))]

/** Whole hours of a duration such as "2h0m0s" or "2h"; null for anything that is not a whole number of hours. */
export function hoursOf(duration: string): number | null {
  const m = /^(\d+)h(?:0m)?(?:0s)?$/.exec(duration)
  return m ? Number(m[1]) : null
}

export const hoursToDuration = (hours: number): string => `${hours}h`

/** The topic as the form edits it: searches and feeds one per line. */
export function topicFields(t: BriefTopic | null): { name: string; category: string; searches: string; region: string; feeds: string; ignore: string } {
  return {
    name: t?.name ?? '',
    category: t?.category ?? '',
    searches: (t?.queries ?? []).join('\n'),
    region: t?.region ?? '',
    feeds: (t?.sources ?? []).filter((s) => s.kind === 'feed').map((s) => s.target).join('\n'),
    ignore: (t?.ignore ?? []).join(', '),
  }
}

/**
 * What the form sends for a topic. A topic's other kinds of source (Hacker News, Mastodon), made by the assistant, are kept as they were: this form
 * edits only searches and feeds, and saving must not drop the rest.
 */
export function topicEdit(f: ReturnType<typeof topicFields>, was: BriefTopic | null): BriefTopicEdit {
  const kept = (was?.sources ?? []).filter((s) => s.kind !== 'feed')
  return {
    name: f.name.trim(),
    category: f.category.trim(),
    queries: fromLines(f.searches),
    region: f.region.trim(),
    sources: [...fromLines(f.feeds).map((target) => ({ kind: 'feed' as const, target })), ...kept],
    ignore: fromWords(f.ignore),
  }
}

export function editionEdit(f: { name: string; readyBy: string; days: number[]; topics: number[]; enabled: boolean; every: number; forHours: number }): BriefEditionEdit {
  return {
    name: f.name.trim(),
    ready_by: f.readyBy,
    days: f.days,
    enabled: f.enabled,
    collect_every: hoursToDuration(f.every),
    collect_for: hoursToDuration(f.forHours),
    sections: f.topics.map((topic_id) => ({ kind: 'news' as const, topic_id })),
  }
}

export const editionTopicIds = (e: BriefEdition | null): number[] => (e?.sections ?? []).map((s) => s.topic_id)
