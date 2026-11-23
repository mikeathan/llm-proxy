import type { BriefEdition, BriefEditionEdit, BriefSource, BriefTopic, BriefTopicEdit, NewsroomSource } from '../../types/newsroom'

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

/** The topic as the form edits it: searches, feed addresses and skip words as lists. */
export function topicFields(t: BriefTopic | null): { name: string; searches: string[]; region: string; feeds: string[]; ignore: string[] } {
  return {
    name: t?.name ?? '',
    searches: [...(t?.queries ?? [])],
    region: t?.region ?? '',
    feeds: (t?.sources ?? []).filter((s) => s.kind === 'feed').map((s) => s.target),
    ignore: [...(t?.ignore ?? [])],
  }
}

/**
 * What the form sends for a topic. A topic's other kinds of source (Hacker News, Mastodon), made by the assistant, are kept as they were: this form
 * edits only searches and feeds, and saving must not drop the rest.
 */
export function topicEdit(f: ReturnType<typeof topicFields>, was: BriefTopic | null, labels: Readonly<Record<string, string>> = {}): BriefTopicEdit {
  const kept = (was?.sources ?? []).filter((s) => s.kind !== 'feed')
  const wasLabel = new Map((was?.sources ?? []).filter((s) => s.label).map((s) => [s.target, s.label as string]))
  return {
    name: f.name.trim(),
    // The form has no category field; a topic's own is kept as it was.
    category: was?.category ?? '',
    queries: [...new Set(f.searches.map((s) => s.trim()).filter((s) => s !== ''))],
    region: f.region.trim(),
    // A feed keeps the name the owner picked it by (the list's, else the one it already had), so the screen can say "BBC News" and not an address.
    sources: [...[...new Set(f.feeds.map((x) => x.trim()).filter((x) => x !== ''))].map((target) => feedSource(target, labels[target] ?? wasLabel.get(target))), ...kept],
    ignore: fromWords(f.ignore.join(',')),
  }
}

const feedSource = (target: string, label: string | undefined): NewsroomSource => (label ? { kind: 'feed', label, target } : { kind: 'feed', target })

/**
 * The groups the source browser shows, the built-in list's order kept, with the owner's own last. A group whose name shares its first four letters with a
 * word of the topic's name goes first, so a "Financial" topic opens on "Business and finance".
 */
export function groupSources(entries: readonly BriefSource[], topicName: string, order: readonly string[]): { category: string; entries: BriefSource[] }[] {
  const stems = topicName.toLowerCase().split(/[^a-z]+/).filter((w) => w.length >= 4).map((w) => w.slice(0, 4))
  const matches = (category: string): boolean => category.toLowerCase().split(/[^a-z]+/).some((w) => stems.includes(w.slice(0, 4)))
  const known = [...order, ...entries.map((e) => e.category).filter((c) => !order.includes(c))]
  const categories = [...new Set(known)]
  const ranked = [...categories.filter(matches), ...categories.filter((c) => !matches(c))]
  return ranked.map((category) => ({ category, entries: entries.filter((e) => e.category === category) })).filter((g) => g.entries.length > 0)
}

export function editionEdit(f: { name: string; readyBy: string; days: number[]; topics: number[]; enabled: boolean; every: number; forHours: number; watches: boolean; home: boolean }): BriefEditionEdit {
  return {
    name: f.name.trim(),
    ready_by: f.readyBy,
    days: f.days,
    enabled: f.enabled,
    watches: f.watches,
    home: f.home,
    collect_every: hoursToDuration(f.every),
    collect_for: hoursToDuration(f.forHours),
    sections: f.topics.map((topic_id) => ({ kind: 'news' as const, topic_id })),
  }
}

export const editionTopicIds = (e: BriefEdition | null): number[] => (e?.sections ?? []).map((s) => s.topic_id)
