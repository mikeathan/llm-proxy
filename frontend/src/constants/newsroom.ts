import type { SourceKind, SourceKindInfo, TopicStatus } from '../types/newsroom'
import type { ChoiceOption, StatusState } from '../types/ui'

// The words of the Newsroom screens.

export const NEWSROOM_TITLE = 'Newsroom'
export const NEWSROOM_SUBTITLE = 'Topics you follow, and what is new in them. Ask the assistant to follow one, or add one here.'
export const NEWSROOM_TAB_LABELS = { feed: 'Feed', topics: 'Topics', sources: 'Sources', settings: 'Settings' } as const
export const NEWSROOM_LOAD_FAILED = 'The Newsroom could not be read.'
export const NEWSROOM_ACTION_FAILED = 'The change could not be made.'
export const NEWSROOM_RELOAD_HINT = 'Check that the service is running, then reload the page.'

export const FEED_EMPTY_TITLE = 'Nothing new yet'
export const FEED_EMPTY_BODY = 'A topic shows its changes here. A topic you have just added records what is already there first and announces nothing.'
export const NO_TOPICS_TITLE = 'You follow nothing yet'
export const NO_TOPICS_BODY = 'Add a topic, or ask the assistant: "keep an eye on new Go releases".'
export const NO_TOPICS_ACTION = 'Add a topic'
export const FILTER_ALL_TOPICS = 'All topics'
export const FILTER_TOPIC_LABEL = 'Show one topic'
export const FILTER_UNREAD_LABEL = 'Unread only'
export const MARK_READ_LABEL = 'Mark read'
export const EARLIER_LABEL = 'Earlier today'
export const CHIP_NEW = 'New'
export const CHIP_CHANGED = 'Changed'
export const ALSO_SEEN_LABEL = 'Also seen'
export const UNREAD_LABEL = 'Unread'

export const SOURCES_TITLE = 'Where the Newsroom reads'
export const CHECK_SOURCE_LABEL = 'Check now'
export const readByWords = (names: readonly string[]): string => `Read for: ${names.join(', ')}`
export const NO_SOURCES_TITLE = 'No sources yet'
export const NO_SOURCES_BODY = 'A source belongs to a topic. Add a topic and give it somewhere to read.'
/** What a source's state says. It is the last check of the topics that read it; a paused or never-checked source has not failed. */
export const SOURCE_STATE_TAG: Record<'failing' | 'working' | 'unchecked' | 'paused', { state: StatusState; label: string }> = {
  failing: { state: 'error', label: 'Failing' },
  working: { state: 'success', label: 'Working' },
  unchecked: { state: 'neutral', label: 'Not checked yet' },
  paused: { state: 'neutral', label: 'Paused' },
}

export const CARD_NO_TOPICS = 'You follow nothing yet. Ask the assistant to keep an eye on something.'
export const CARD_HEADLINES_EMPTY = 'Nothing new yet.'
export const CARD_FAILING_WORDS = (names: readonly string[]): string => `${names.join(', ')} cannot read a source.`
export const NEWSROOM_CARD_RELOAD = 'It is read again in a minute.'

export const NEW_TOPIC_LABEL = 'New topic'
export const TOPIC_NAME_LABEL = 'Name'
export const TOPIC_NAME_MAX = 80
export const TOPIC_INTERVAL_LABEL = 'Check every'
export const TOPIC_CHANNEL_LABEL = 'Send a message when something is new'
export const TOPIC_CHANNEL_HINT = 'Every change is kept in the feed either way. Pick a channel to be told as well.'
export const TOPIC_FEED_ONLY = 'No, the feed only'
export const TOPIC_ON_LABEL = 'Following'
export const TOPIC_SOURCES_LABEL = 'Sources'
export const MAX_SOURCES = 10
export const ADD_SOURCE_LABEL = 'Add a source'
export const REMOVE_SOURCE_LABEL = 'Remove source'
export const SOURCE_KIND_LABEL = 'Kind'
export const SOURCE_INSTANCE_LABEL = 'Server'
export const SOURCE_INSTANCE_HINT = 'For example mastodon.social'
export const RUN_NOW_LABEL = 'Check now'
export const PAUSED_LABEL = 'Paused'
export const FEED_ONLY_WORDS = 'Feed only'
export const sendsToWords = (channel: string): string => `Sends to ${channel}`
export const NEVER_CHECKED = 'Not checked yet'
export const lastCheckedWords = (relative: string): string => `Checked ${relative}`
export const updatesWords = (n: number): string => `${n} ${n === 1 ? 'update' : 'updates'}`
export const dayHeadline = (updates: number, last: string): string => `${updatesWords(updates)}, last ${last}`
export const runDoneWords = (found: number): string => (found === 0 ? 'Checked: nothing new' : `Checked: ${found} new`)

export const deleteTopicTitle = 'Stop following this topic?'
export const deleteTopicMessage = (name: string): string => `"${name}" and what it found are removed from the feed.`

export const INTERVAL_OPTIONS: ChoiceOption[] = [
  { value: '5m', label: 'Every 5 minutes' },
  { value: '15m', label: 'Every 15 minutes' },
  { value: '30m', label: 'Every 30 minutes' },
  { value: '1h', label: 'Every hour' },
  { value: '3h', label: 'Every 3 hours' },
  { value: '6h', label: 'Every 6 hours' },
  { value: '12h', label: 'Every 12 hours' },
  { value: '24h', label: 'Every day' },
]
export const DEFAULT_INTERVAL = '1h'

export const SOURCE_KIND_INFO: Record<SourceKind, SourceKindInfo> = {
  feed: { label: 'News feed or site (RSS, Atom, GitHub releases)', targetLabel: 'Address', hint: 'For example https://go.dev/blog/feed.atom' },
  hackernews: { label: 'Hacker News search', targetLabel: 'Search words', hint: 'Stories matching these words' },
  mastodon: { label: 'Mastodon tag', targetLabel: 'Tag', hint: 'The tag without the #' },
  page: { label: 'Web page (told when it changes)', targetLabel: 'Address', hint: 'The page is read and compared by its visible text' },
  telegram: { label: 'Telegram public channel', targetLabel: 'Channel name', hint: 'Reads the channel’s public preview page; adding it is your opt-in' },
}
export const SOURCE_KIND_OPTIONS: ChoiceOption[] = (Object.keys(SOURCE_KIND_INFO) as SourceKind[]).map((k) => ({ value: k, label: SOURCE_KIND_INFO[k].label }))
export const SOURCE_TARGET_MAX = 500

/** What a topic's last run says, in plain words. */
export const TOPIC_STATUS_TAG: Record<TopicStatus, { state: StatusState; label: string }> = {
  baseline: { state: 'info', label: 'Recorded what is there' },
  nothing_new: { state: 'neutral', label: 'Nothing new' },
  in_feed: { state: 'success', label: 'In the feed' },
  sent: { state: 'success', label: 'Sent' },
  send_failed: { state: 'error', label: 'Send failed' },
  no_channel: { state: 'warning', label: 'No channel' },
  analysis_failed: { state: 'warning', label: 'Could not compare' },
  failing: { state: 'error', label: 'Source failing' },
}
