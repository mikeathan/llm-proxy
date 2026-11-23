import { MAX_EDITION_TOPICS } from '../../constants/newsroom'
import type { SectionGroup, BriefEdition, BriefEditionEdit, BriefSource, BriefStarter, BriefTopic, BriefTopicEdit, NewsroomSource } from '../../types/newsroom'

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

// The server matches a section without regard to case or extra spaces (brief.CategoryKey); the form does the same so the two never disagree.
const categoryKey = (c: string): string => c.trim().replace(/\s+/g, ' ').toLowerCase()

/** The sections of a list of topics, in the order each first appears; the first topic's spelling names the section. */
export function sectionGroups(topics: readonly BriefTopic[]): SectionGroup[] {
  const out: SectionGroup[] = []
  for (const t of topics) {
    const category = t.category?.trim() ?? ''
    const key = category === '' ? `topic:${t.id}` : `section:${categoryKey(category)}`
    const seen = out.find((g) => g.key === key)
    if (seen) seen.topics.push(t)
    else out.push({ key, label: category === '' ? t.name : category.replace(/\s+/g, ' '), topics: [t], own: category === '' })
  }
  return out
}

/** The sections an edition reads: those with at least one of its topics. */
export function editionSectionKeys(e: BriefEdition | null, groups: readonly SectionGroup[]): string[] {
  const ids = new Set(editionTopicIds(e))
  return groups.filter((g) => g.topics.some((t) => ids.has(t.id))).map((g) => g.key)
}

/** The topic ids of the picked sections, in the order the sections are listed. */
export const topicIdsOf = (keys: readonly string[], groups: readonly SectionGroup[]): number[] => groups.filter((g) => keys.includes(g.key)).flatMap((g) => g.topics.map((t) => t.id))

/** What an edition reads, in the order the brief is written: its news sections, then watches, then home. */
export const editionBlocks = (e: BriefEdition, groups: readonly SectionGroup[]): string[] => [
  ...groups.filter((g) => editionSectionKeys(e, groups).includes(g.key)).map((g) => g.label),
  ...(e.watches ? ['Watches'] : []),
  ...(e.home ? ['Home'] : []),
]

/**
 * The edit that adds a new topic to an edition that already reads the topic's section, so a topic added to News shows up in the briefs that carry News. Null
 * when the edition reads no other topic of that section, the topic has none, or the edition is at the server's limit of topics.
 */
export function editionJoining(e: BriefEdition, topics: readonly BriefTopic[], added: BriefTopic): BriefEditionEdit | null {
  const category = added.category?.trim() ?? ''
  if (category === '' || e.sections.length >= MAX_EDITION_TOPICS) return null
  const ids = new Set(e.sections.map((s) => s.topic_id))
  const reads = topics.some((t) => ids.has(t.id) && t.category && categoryKey(t.category) === categoryKey(category))
  if (!reads || ids.has(added.id)) return null
  return {
    name: e.name, ready_by: e.ready_by, days: e.days, enabled: e.enabled, watches: !!e.watches, home: !!e.home,
    collect_every: e.collect_every, collect_for: e.collect_for,
    sections: [...e.sections, { kind: 'news', topic_id: added.id }],
  }
}

/** A topic made from one typed word inside a section: the word is its name and its search. */
export const quickTopic = (word: string, section: string): BriefTopicEdit => ({ name: word.trim(), category: section.trim(), queries: [word.trim()], region: '', sources: [], ignore: [] })

/** What a topic reads besides its searches, by the names it was picked by. */
export const topicReads = (t: BriefTopic): string[] => (t.sources ?? []).map((s) => s.label || s.target)

/** A topic that is one short word and nothing else brings a flood of loosely related stories. */
export const isThinTopic = (t: BriefTopic): boolean => {
  const q = t.queries ?? []
  const word = q[0]?.trim() ?? ''
  return q.length === 1 && !/\s/.test(word) && word.length < 9 && (t.sources ?? []).length === 0
}
