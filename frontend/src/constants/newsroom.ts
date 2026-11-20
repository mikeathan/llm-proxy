import type { HelpSection, SourceGuide, SourceKind, SourceKindInfo, TopicStatus } from '../types/newsroom'
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
  telegram: { label: 'Telegram public channel', targetLabel: 'Channel name', hint: 'The name or the link, such as aipost or t.me/aipost. Adding it is your opt-in' },
}
export const SOURCE_KIND_OPTIONS: ChoiceOption[] = (Object.keys(SOURCE_KIND_INFO) as SourceKind[]).map((k) => ({ value: k, label: SOURCE_KIND_INFO[k].label }))
export const SOURCE_TARGET_MAX = 500
/** Why a source's words are refused, per kind. The server checks the same rules again (stacks/newsroom/sources/validate.go). */
export const SOURCE_PROBLEMS = {
  address: 'Use a full https:// address, such as https://example.com/feed.',
  query: 'Type 1 to 200 characters of search words, such as rust async.',
  channel: 'A channel name is 4 to 32 letters, digits or underscores, starting with a letter, such as aipost.',
  tag: 'A tag is letters, digits or underscores with no spaces, such as golang.',
  instance: 'The server is a host name such as mastodon.social.',
  instanceMissing: 'Type the Mastodon server, such as mastodon.social.',
} as const
export const HN_QUERY_MAX = 200

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

export const HELP_TITLE = 'Help'
export const HELP_INTRO = 'You follow topics. A topic is a name plus the places it reads. The Newsroom reads them on a timer and keeps only what is new in the Feed.'
export const HELP_STEPS_TITLE = 'Follow something'
export const HELP_STEPS: readonly string[] = [
  'Topics tab, then New topic. Give it a name and add a source.',
  'Save, then press Check now. The first check only records what is already there and tells you nothing.',
  'Later checks add what is new to the Feed. Pick a channel on the topic if you also want a message.',
]
export const HELP_SOURCES_TITLE = 'Sources you can add'
export const HELP_SOURCES_INTRO = 'Pick a source to see how to fill it in.'
export const GUIDE_STEPS_LABEL = 'How to fill it in'
export const GUIDE_EXAMPLE_LABEL = 'Example'
export const GUIDE_SHOW_LABEL = 'How do I find this?'
export const SOURCE_GUIDES: Record<SourceKind, SourceGuide> = {
  feed: {
    name: 'News site or feed',
    goodFor: 'Blogs, news sites, podcasts and GitHub releases.',
    steps: [
      'Open the site and look for an RSS or Atom link. It is often an orange icon, or in the footer.',
      'No link? Add /feed, /rss or /atom.xml to the end of the site address.',
      'Paste the feed address. It must start with https://.',
      'For GitHub releases use https://github.com/OWNER/REPO/releases.atom with the owner and project filled in.',
    ],
    example: 'https://go.dev/blog/feed.atom',
  },
  hackernews: {
    name: 'Hacker News search',
    goodFor: 'New stories on Hacker News about words you choose.',
    steps: ['Type one to three words that describe what you care about.', 'Pick specific words. A broad word such as ai finds new stories at almost every check.'],
    example: 'rust async',
  },
  mastodon: {
    name: 'Mastodon tag',
    goodFor: 'Public posts that use a tag on one Mastodon server.',
    steps: ['In Server, type the Mastodon site you want to read, such as mastodon.social.', 'In Tag, type the tag without the #.'],
    example: 'Server mastodon.social, tag golang',
  },
  page: {
    name: 'Web page',
    goodFor: 'A page with no feed: a status page, a changelog, a price.',
    steps: [
      'Open the page in your browser and copy its address.',
      'Paste it. The first check records the page as it is now.',
      'You are told when its visible text changes.',
    ],
    example: 'https://status.example.com/',
    note: 'A page that shows the time or a counter changes at every check.',
  },
  telegram: {
    name: 'Telegram channel',
    goodFor: 'Posts from a public Telegram channel.',
    steps: [
      'In Telegram open the channel, tap its name and copy its link. It looks like t.me/aipost.',
      'Paste the link or just the name. Both work.',
      'To be sure, open t.me/s/NAME in a browser first. If recent posts show there, it works here.',
    ],
    example: 't.me/aipost',
    note: 'A private channel, or one that has turned the web preview off, cannot be read. A channel is read at most every 10 minutes.',
  },
}

export const HELP_GOOD_TITLE = 'Good to know'
export const HELP_GOOD_TO_KNOW: readonly HelpSection[] = [
  { title: 'Feed only', lines: ['Changes are kept in the Feed and nothing is sent. Choose a channel on a topic to be messaged as well.'] },
  { title: 'Source failing', lines: ['The address could not be read. Open it in a browser, and check that this computer can reach the internet.'] },
  { title: 'Could not compare', lines: ['No model is chosen, so every item is announced as new instead of being compared with what you saw before.'] },
  { title: 'Deleting a topic', lines: ['It removes what the topic found from the Feed.'] },
]
