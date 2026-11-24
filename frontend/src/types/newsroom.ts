// The Newsroom as /admin/api/newsroom/* reports it (stacks/newsroom: watches, the watch view and the feed view model).

export const SOURCE_KINDS = ['feed', 'hackernews', 'mastodon', 'newssearch', 'page', 'telegram'] as const
export type SourceKind = (typeof SOURCE_KINDS)[number]

export interface NewsroomSource {
  kind: SourceKind
  target: string
  label?: string
  /** A Mastodon source's server, such as mastodon.social; a news search's region, such as en-GB. */
  instance?: string
}

/** The words the watch form shows for one kind of source. */
export interface SourceKindInfo {
  label: string
  targetLabel: string
  hint: string
}

/** What a watch's last run did, in the server's words. */
export type WatchStatus = 'baseline' | 'nothing_new' | 'in_feed' | 'sent' | 'send_failed' | 'no_channel' | 'analysis_failed' | 'failing'

export interface NewsroomWatch {
  id: number
  name: string
  sources: NewsroomSource[]
  /** A duration such as "1h0m0s". */
  interval: string
  /** How old a post may be and still be news, a duration such as "72h0m0s". */
  max_age: string
  enabled: boolean
  /** What the owner wants to know about, in their words; absent means every new item counts. */
  look_for?: string
  /** An item must contain one of these to be looked at (lower case), and is dropped if it contains one of skip. */
  keywords?: string[]
  skip?: string[]
  /** The connector type notices go through; absent means the feed only. */
  channel?: string
  last_run?: string
  last_status?: WatchStatus
  last_detail?: string
  /** One sentence per source the last check could not read; kept while the check itself answered from the others. */
  last_problems?: string[]
  last_new?: number
}

/** What a create or a save sends. The interval is a duration such as "1h". */
export interface NewsroomWatchEdit {
  name: string
  sources: NewsroomSource[]
  interval: string
  max_age: string
  enabled: boolean
  channel: string
  look_for: string
  keywords: string[]
  skip: string[]
}

export interface NewsroomAlsoSeen {
  source: string
  link?: string
  at: string
}

export interface NewsroomEntry {
  id: number
  watch_id: number
  at: string
  source: string
  title: string
  link?: string
  text?: string
  /** When the source says the post was published; absent when it gave no date. `at` is when the watch found it. */
  posted?: string
  /** The model's short reason the item matches what the watch looks for. */
  why?: string
  /** The model's own short headline (shown in place of the title) and one-or-two-sentence summary (shown when opened). The original title, link and text are kept. */
  headline?: string
  summary?: string
  /** True for an item shown from the watch's first check: it was already there, so nothing was announced. */
  first?: boolean
  /** A changed fact, when the model step found one. */
  was?: string
  now?: string
  also_seen?: NewsroomAlsoSeen[]
  sent?: boolean
  read?: boolean
}

/** One watch's findings on one day: the newest first, every earlier one beneath it. */
export interface NewsroomWatchDay {
  watch_id: number
  watch: string
  updates: number
  unread: number
  current: NewsroomEntry
  earlier?: NewsroomEntry[]
  last_at: string
}

export interface NewsroomDay {
  /** YYYY-MM-DD in the zone the screen asked for. */
  day: string
  label: string
  updates: number
  unread: number
  watches: NewsroomWatchDay[]
}

export interface NewsroomFeedFilter {
  watch: number | null
  unreadOnly: boolean
  /** Only the last N calendar days (1 is today); null shows everything the feed keeps (30 days). */
  days: number | null
}

export interface NewsroomRunOutcome {
  watch_id: number
  /** "waiting" answers a manual check made inside a source's minimum gap between reads; it is never stored on a watch. */
  status: WatchStatus | 'waiting'
  detail: string
  new: number
}

/** A finding with the watch it belongs to, for a list that mixes watches. */
export interface Headline {
  watch: string
  entry: NewsroomEntry
}

/** One item of a source preview. */
export interface SourceSample {
  title: string
  link?: string
  text?: string
  at?: string
}

/** Something the owner can pick to fill a source: a feed a page advertises, or a tag a server lists. */
export interface SourceChoice {
  value: string
  label: string
  note?: string
}

/** What a look-up found (stacks/newsroom/sources/lookup.go). target and instance are the cleaned-up values a save would keep. */
export interface SourcePreview {
  found: boolean
  target?: string
  instance?: string
  title?: string
  sample: SourceSample[]
  choices: SourceChoice[]
  problem?: string
}

/** The tags a Mastodon server offers while the owner types. */
export interface SourceSuggestion {
  choices: SourceChoice[]
  problem?: string
}

/** How to use one kind of source: what it is for, the steps to fill it in, and an example of what to type. */
export interface SourceGuide {
  name: string
  goodFor: string
  steps: readonly string[]
  example: string
  note?: string
}

// The Briefs module as /admin/api/newsroom/briefs/* reports it (stacks/newsroom/brief: editions, topics and the editions already made).

/** One story of an edition. Everything but the headline and summary is the collected story's own data. */
/** Which bundled artwork a section gets. */
export type PlateKind = 'world' | 'economy' | 'tech' | 'politics' | 'science' | 'paper' | 'watch' | 'home'

export interface BriefStory {
  pool_id: number
  headline: string
  /** Empty for a story with no source text: nothing was written about it. */
  summary?: string
  link?: string
  posted?: string
  /** Each outlet that reported it. */
  outlets: string[]
  headline_only?: boolean
  /** Shown before, and more outlets report it now. */
  update?: boolean
}

export interface BriefSection {
  kind: 'news'
  topic_id: number
  name: string
  category?: string
  stories: BriefStory[]
  /** When the topic was last collected. */
  as_of?: string
  /** What the reader should know: stale, never collected, or not written by the model. */
  note?: string
  /** One sentence per source that could not be read. */
  problems?: string[]
}

/** One watch's part of an edition: how many findings, the newest, and the sources it could not read. */
export interface BriefWatchLine {
  watch: string
  new: number
  headline?: string
  summary?: string
  link?: string
  at?: string
  problems?: string[]
}

/** One fact of the Home block. */
export interface BriefHomeLine {
  id: string
  count?: number
  words: string
}

/** An edition as it was made; reading one asks nothing of the model. */
export interface BriefIssue {
  id: number
  edition_id: number
  name: string
  made_at: string
  /** The moment it was made for (its ready time); absent for one written by hand. */
  ready_for?: string
  /** When the owner first read it; absent while unread. */
  read_at?: string
  top?: BriefStory[]
  sections: BriefSection[]
  /** What the watches found since the previous edition; absent when the edition does not ask. */
  watches?: BriefWatchLine[]
  /** The core's fixed facts about the network and Radio: a count and the core's own sentence for an id; absent when the edition does not ask. */
  home?: BriefHomeLine[]
  /** How many different outlets reported the stories the edition could choose from; absent on older editions. */
  outlets_read?: number
  model_calls: number
}

/** The newest edition made for one edition and whether the owner read it: what a tab's dot and the dashboard card ask, without the edition itself. */
export interface BriefLatest {
  edition_id: number
  name: string
  issue_id: number
  made_at: string
  read: boolean
}

export interface BriefEdition {
  id: number
  name: string
  /** A time of day such as "07:00", read in the stack's timezone. */
  ready_by: string
  /** Weekdays it is made on, 0 is Sunday. */
  days: number[]
  /** Durations such as "2h0m0s". */
  collect_every: string
  collect_for: string
  sections: { kind: 'news'; topic_id: number }[]
  enabled: boolean
  /** Adds the Your watches block. */
  watches?: boolean
  /** Adds the Home block: the core's counts about the network and Radio. */
  home?: boolean
}

export interface BriefTopic {
  id: number
  name: string
  category?: string
  queries?: string[]
  region?: string
  sources?: NewsroomSource[]
  ignore?: string[]
  last_collected?: string
  last_new?: number
  last_problems?: string[]
}

/** A ready-made topic: its name and section, its searches, and well-known sites named as the built-in list names them. */
export interface BriefStarter {
  name: string
  section: string
  queries: readonly string[]
  sites: readonly string[]
}

/** One source the topic form offers: the built-in list (builtin) and the owner's own (an id, so it can be removed). A feed target is an address; a news search's is the search. */
export interface BriefSource {
  id?: number
  label: string
  category: string
  kind: 'feed' | 'newssearch'
  target: string
  builtin: boolean
}

/** The stack's daily limit of model calls (watches and briefs together): the owner's setting, 0 meaning the default, and the bounds. */
export interface BriefSettings {
  model_calls_per_day: number
  default: number
  max: number
}

/** What a topic create or save sends. */
export interface BriefTopicEdit {
  name: string
  category: string
  queries: string[]
  region: string
  sources: NewsroomSource[]
  ignore: string[]
}

/** What an edition create or save sends. The durations are strings such as "2h". */
export interface BriefEditionEdit {
  name: string
  ready_by: string
  days: number[]
  enabled: boolean
  watches: boolean
  home: boolean
  collect_every: string
  collect_for: string
  sections: { kind: 'news'; topic_id: number }[]
}

/** A section of the brief as the owner sees it: the topics that share a section name, or one topic that has none (its own name is its section). */
export interface SectionGroup {
  key: string
  label: string
  topics: BriefTopic[]
  own: boolean
}

/** What the Briefs page says first about the brief on screen: is it today's, and does anything need pressing? */
export interface BriefStatus {
  /** 'fresh' is today's brief; 'old' is from an earlier day and says so loudly; 'none' has no brief yet. */
  tone: 'fresh' | 'old' | 'none'
  title: string
  detail: string
}
