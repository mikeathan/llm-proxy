// The Newsroom as /admin/api/newsroom/* reports it (stacks/newsroom: watches, the watch view and the feed view model).

export const SOURCE_KINDS = ['feed', 'hackernews', 'mastodon', 'page', 'telegram'] as const
export type SourceKind = (typeof SOURCE_KINDS)[number]

export interface NewsroomSource {
  kind: SourceKind
  target: string
  label?: string
  /** A Mastodon source's server, such as mastodon.social. */
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
  last_new?: number
}

/** What a create or a save sends. The interval is a duration such as "1h". */
export interface NewsroomWatchEdit {
  name: string
  sources: NewsroomSource[]
  interval: string
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
