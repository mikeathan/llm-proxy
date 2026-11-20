// The Newsroom as /admin/api/newsroom/* reports it (stacks/newsroom: topics, the topic view and the feed view model).

export const SOURCE_KINDS = ['feed', 'hackernews', 'mastodon', 'page', 'telegram'] as const
export type SourceKind = (typeof SOURCE_KINDS)[number]

export interface NewsroomSource {
  kind: SourceKind
  target: string
  label?: string
  /** A Mastodon source's server, such as mastodon.social. */
  instance?: string
}

/** The words the topic form shows for one kind of source. */
export interface SourceKindInfo {
  label: string
  targetLabel: string
  hint: string
}

/** What a topic's last run did, in the server's words. */
export type TopicStatus = 'baseline' | 'nothing_new' | 'in_feed' | 'sent' | 'send_failed' | 'no_channel' | 'analysis_failed' | 'failing'

export interface NewsroomTopic {
  id: number
  name: string
  sources: NewsroomSource[]
  /** A duration such as "1h0m0s". */
  interval: string
  enabled: boolean
  /** The connector type notices go through; absent means the feed only. */
  channel?: string
  last_run?: string
  last_status?: TopicStatus
  last_detail?: string
  last_new?: number
}

/** What a create or a save sends. The interval is a duration such as "1h". */
export interface NewsroomTopicEdit {
  name: string
  sources: NewsroomSource[]
  interval: string
  enabled: boolean
  channel: string
}

export interface NewsroomAlsoSeen {
  source: string
  link?: string
  at: string
}

export interface NewsroomEntry {
  id: number
  topic_id: number
  at: string
  source: string
  title: string
  link?: string
  text?: string
  /** A changed fact, when the model step found one. */
  was?: string
  now?: string
  also_seen?: NewsroomAlsoSeen[]
  sent?: boolean
  read?: boolean
}

/** One topic's findings on one day: the newest first, every earlier one beneath it. */
export interface NewsroomTopicDay {
  topic_id: number
  topic: string
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
  topics: NewsroomTopicDay[]
}

export interface NewsroomFeedFilter {
  topic: number | null
  unreadOnly: boolean
}

export interface NewsroomRunOutcome {
  topic_id: number
  status: TopicStatus
  detail: string
  new: number
}

/** A finding with the topic it belongs to, for a list that mixes topics. */
export interface Headline {
  topic: string
  entry: NewsroomEntry
}

/** One place the Newsroom reads, with the topics that read it. The status is the last check of those topics: the server keeps one per topic, not one per source. */
export interface SourceRow {
  key: string
  source: NewsroomSource
  topics: NewsroomTopic[]
  state: 'failing' | 'working' | 'unchecked' | 'paused'
  /** The server's words for the failure, when there is one. */
  detail: string
}

/** One block of the Newsroom Help tab. */
export interface HelpSection { title: string; lines: readonly string[] }
