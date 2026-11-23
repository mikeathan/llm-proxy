import type { BriefEdition, BriefEditionEdit, BriefSource, BriefStarter, BriefTopic, BriefTopicEdit, NewsroomSource } from '../../types/newsroom'

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
export function topicFields(t: BriefTopic | null): { name: string; category: string; searches: string[]; region: string; feeds: string[]; ignore: string[] } {
  return {
    name: t?.name ?? '',
    category: t?.category ?? '',
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
    category: f.category.trim(),
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

/** The built-in feeds a starter names, in its order; a name the list does not hold (the list not loaded yet, or changed) is left out. */
export function starterFeeds(s: BriefStarter, sources: readonly BriefSource[]): BriefSource[] {
  return s.sites.map((label) => sources.find((x) => x.kind === 'feed' && x.label === label)).filter((x): x is BriefSource => x !== undefined)
}

/** The starters that can be read now: each has a search or a site found in the list. */
export const usableStarters = (all: readonly BriefStarter[], sources: readonly BriefSource[]): BriefStarter[] =>
  all.filter((s) => s.queries.length > 0 || starterFeeds(s, sources).length > 0)

/** What a starter saves as a topic, its sites copied in with their names like any site the owner picked. */
export const starterEdit = (s: BriefStarter, sources: readonly BriefSource[]): BriefTopicEdit => ({
  name: s.name,
  category: s.section,
  queries: [...s.queries],
  region: '',
  sources: starterFeeds(s, sources).map((x) => feedSource(x.target, x.label)),
  ignore: [],
})

const upperFirst = (t: string): string => t.charAt(0).toUpperCase() + t.slice(1)
const hostOf = (address: string): string => {
  try {
    return new URL(address).hostname.replace(/^www\./, '')
  } catch {
    return ''
  }
}

/** The name a topic gets when none is typed: its first search (not a publisher search), else its first site by name or host. */
export function topicNameFrom(searches: readonly string[], feeds: readonly string[], labels: Readonly<Record<string, string>>): string {
  const search = searches.map((s) => s.trim()).find((s) => s !== '' && !/^site:/i.test(s))
  if (search) return upperFirst(search)
  const feed = feeds.map((x) => labels[x] ?? hostOf(x)).find((x) => x !== '')
  return feed ?? ''
}
