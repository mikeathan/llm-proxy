import type { HelpSection, SourceKind, SourceKindInfo, TopicStatus } from '../types/newsroom'
import type { ChoiceOption, StatusState } from '../types/ui'

// The words of the Newsroom screens.

export const NEWSROOM_TITLE = 'Newsroom'
export const NEWSROOM_SUBTITLE = 'Topics you follow, and what is new in them. Ask the assistant to follow one, or add one here.'
export const NEWSROOM_TAB_LABELS = { feed: 'Feed', topics: 'Topics', sources: 'Sources', settings: 'Settings', help: 'Help' } as const
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
export const TOPIC_NAME_HINT = 'Only a label for you, such as "AI news". Where it reads is set under Sources below.'
export const TOPIC_FORM_INTRO = 'A topic is something you follow. Give it a name, then say where to read about it.'
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

export const HELP_TITLE = 'How the Newsroom works'
/** The words of the Help tab: what a topic is, where to find sources, and what the states mean. Plain text only. */
export const HELP_SECTIONS: readonly HelpSection[] = [
  {
    title: 'What it does',
    lines: [
      'You follow topics. Each topic reads the places you give it on a timer and keeps what is new in the Feed.',
      'A topic is a name plus its sources. The name is only a label for you; the sources are where it reads. It follows everything a source publishes, so pick sources that are about what you care about.',
    ],
  },
  {
    title: 'Follow something in four steps',
    lines: [
      '1. Topics tab, New topic. Type a name, then add a source (see the next section).',
      '2. Save. Press Check now on the topic. The first check only records what is there already and tells you nothing.',
      '3. Wait for something new, then press Check now again, or let the timer do it.',
      '4. Open the Feed. New items appear there. Pick a channel on the topic if you also want a message.',
    ],
  },
  {
    title: 'Where to find sources',
    lines: [
      'News feed or site: the address of its feed, such as https://go.dev/blog/feed.atom. Try /feed, /rss or /atom.xml on a site\'s address, or look for an RSS link on the page.',
      'GitHub releases: https://github.com/OWNER/REPO/releases.atom, with the owner and project name filled in.',
      'Hacker News search: words such as llm or rust. Stories that match them are new at almost every check.',
      'Mastodon tag: the tag without the #, and the server it is read from, such as mastodon.social.',
      'Telegram public channel: the channel username, without @ or t.me/. It is the name in the channel\'s share link. Open https://t.me/s/NAME in a browser first: if recent posts show there, it works here. A private channel has no public preview. Telegram is checked every 10 minutes at most.',
      'Web page: any address. You are told when its visible text changes.',
    ],
  },
  {
    title: 'Good to know',
    lines: [
      'Feed only means changes are kept in the Feed and nothing is sent. Choose a channel on a topic to be messaged as well.',
      'Source failing means the address could not be read. Check it in a browser, and check that this computer can reach the internet.',
      'Not compared means no model is chosen, so every item is announced as new instead of being compared with what you saw before.',
      'Deleting a topic removes what it found from the Feed.',
    ],
  },
]
