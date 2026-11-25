import type { BriefFollowEdit, BriefTopic, BriefTopicEdit, NewsroomWatchEdit } from '../../types/newsroom'
import { DEFAULT_INTERVAL, DEFAULT_MAX_AGE, FOLLOW_WORDS_MAX, MAX_TOPIC_IGNORE, MAX_TOPIC_QUERIES, STOP_WORDS, WATCH_NAME_MAX } from '../../constants/newsroom'

/**
 * The words of a headline worth searching for or skipping: lower case, letters and digits only, without the common little words, each once, in the order
 * they appear, at most FOLLOW_WORDS_MAX.
 */
export function storyWords(headline: string): string[] {
  const words = headline.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter((w) => w.length >= 3 && !STOP_WORDS.has(w))
  return [...new Set(words)].slice(0, FOLLOW_WORDS_MAX)
}

/** A topic as an edit, everything it holds sent back unchanged (a save replaces the topic). */
const asEdit = (t: BriefTopic): BriefTopicEdit => ({ name: t.name, category: t.category ?? t.name, queries: [...(t.queries ?? [])], region: t.region ?? '', sources: [...(t.sources ?? [])], ignore: [...(t.ignore ?? [])] })

/**
 * "More like this": the picked words become one more search of the topic. Null when nothing is picked, the search is already there, or the topic is at the
 * server's limit of searches.
 */
export function moreLikeThis(t: BriefTopic, words: string[]): BriefTopicEdit | null {
  const search = words.join(' ').trim()
  const edit = asEdit(t)
  if (!search || edit.queries.some((q) => q.toLowerCase() === search) || edit.queries.length >= MAX_TOPIC_QUERIES) return null
  return { ...edit, queries: [...edit.queries, search] }
}

/**
 * "Less like this": each picked word is skipped from now on (a story whose headline or text holds it is not collected). Null when no new word is picked or
 * the topic would pass the server's limit of skip words.
 */
export function lessLikeThis(t: BriefTopic, words: string[]): BriefTopicEdit | null {
  const edit = asEdit(t)
  const added = words.map((w) => w.toLowerCase()).filter((w) => w && !edit.ignore.includes(w))
  if (!added.length || edit.ignore.length + added.length > MAX_TOPIC_IGNORE) return null
  return { ...edit, ignore: [...edit.ignore, ...added] }
}

/** What following something as a brief topic sends: its words as the search and the name, in the section picked, with a feed when one was given. */
export function followTopic(f: BriefFollowEdit): BriefTopicEdit {
  const text = f.text.trim()
  return { name: text.slice(0, WATCH_NAME_MAX), category: f.section.trim() || text.slice(0, WATCH_NAME_MAX), queries: [text], region: '', sources: f.feed ? [{ kind: 'feed', target: f.feed.trim() }] : [], ignore: [] }
}

/** What following something as an alert sends: a news search for its words (and a feed when one was given), checked every hour. */
export function followWatch(f: BriefFollowEdit): NewsroomWatchEdit {
  const text = f.text.trim()
  return {
    name: text.slice(0, WATCH_NAME_MAX),
    sources: [{ kind: 'newssearch', target: text }, ...(f.feed ? [{ kind: 'feed' as const, target: f.feed.trim() }] : [])],
    interval: DEFAULT_INTERVAL, max_age: DEFAULT_MAX_AGE, enabled: true, channel: f.channel, look_for: f.lookFor.trim(), keywords: [], skip: [],
  }
}
