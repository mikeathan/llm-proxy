import type { SourceGuide, SourceKind, SourceKindInfo, WatchStatus } from '../types/newsroom'
import type { ChoiceOption, StatusState } from '../types/ui'

// The words of the Newsroom screens.

export const NEWSROOM_TITLE = 'Newsroom'
export const NEWSROOM_SUBTITLE = 'What is new about the things you care about. Set up a watch once, or ask the assistant to.'
export const NEWSROOM_TAB_LABELS = { feed: 'Feed', watches: 'Watches', settings: 'Settings' } as const
export const NEWSROOM_LOAD_FAILED = 'The Newsroom could not be read.'
export const NEWSROOM_ACTION_FAILED = 'The change could not be made.'
export const NEWSROOM_RELOAD_HINT = 'Check that the service is running, then reload the page.'

export const FEED_EMPTY_TITLE = 'Nothing new yet'
export const FEED_EMPTY_BODY = 'What a watch finds shows up here. A new watch shows a few items that were already there, marked "First check", and tells you only about what comes after.'
export const NO_WATCHES_TITLE = 'You follow nothing yet'
export const NO_WATCHES_BODY = 'Add a watch, or ask the assistant: "keep an eye on new Go releases".'
export const NO_WATCHES_ACTION = 'Add a watch'
export const FILTER_ALL_WATCHES = 'All watches'
export const FILTER_WATCH_LABEL = 'Show one watch'
export const FILTER_UNREAD_LABEL = 'Unread only'
export const MARK_READ_LABEL = 'Mark read'
export const EARLIER_LABEL = 'Earlier today'
export const CHIP_NEW = 'New'
export const CHIP_CHANGED = 'Changed'
export const ALSO_SEEN_LABEL = 'Also seen'
export const UNREAD_LABEL = 'Unread'

export const CARD_NO_WATCHES = 'You follow nothing yet. Ask the assistant to keep an eye on something.'
export const CARD_HEADLINES_EMPTY = 'Nothing new yet.'
export const CARD_FAILING_WORDS = (names: readonly string[]): string => `${names.join(', ')} cannot read a source.`
export const NEWSROOM_CARD_RELOAD = 'It is read again in a minute.'

export const WATCH_FILTER_ACTIVE = 'looking for'
export const NEW_WATCH_LABEL = 'New watch'
export const WATCH_NAME_LABEL = 'Name'
export const WATCH_NAME_MAX = 80
export const WATCH_NAME_HINT = 'Only a label for you, such as "AI model news".'
export const WATCH_FORM_INTRO = 'A watch is one thing you follow: where to read, what to look for, and how often. Start from a suggestion or write your own.'
export const WATCH_INTERVAL_LABEL = 'Check every'
export const WATCH_CHANNEL_LABEL = 'Send a message when something is new'
export const WATCH_CHANNEL_HINT = 'Every change is kept in the feed either way. Pick a channel to be told as well.'
export const WATCH_FEED_ONLY = 'No, the feed only'
export const WATCH_ON_LABEL = 'Following'
export const WATCH_SOURCES_LABEL = 'Where to read'
export const WATCH_LOOK_FOR_LABEL = 'What to look for'
export const WATCH_LOOK_FOR_HINT = 'In your own words, for example "new open-weight model releases, not ads". The model keeps only items about it and says why. Empty means everything the sources post.'
export const WATCH_LOOK_FOR_MAX = 300
export const WATCH_SUGGESTIONS_LABEL = 'Suggestions'
export const WATCH_KEYWORDS_LABEL = 'Must contain one of (optional)'
export const WATCH_KEYWORDS_HINT = 'Separate with commas. An item without any of these words is dropped before the model looks at it.'
export const WATCH_SKIP_LABEL = 'Skip if it contains (optional)'
export const WATCH_SKIP_HINT = 'Separate with commas, such as sponsored, giveaway.'
/** Starting points for what to look for. Choosing one fills in the name when it is empty and the text; both stay editable. */
export const WATCH_TEMPLATES: readonly { label: string; name: string; lookFor: string }[] = [
  { label: 'Product releases', name: 'Releases', lookFor: 'New versions or releases of the software or products I follow; not announcements of plans.' },
  { label: 'Security problems', name: 'Security', lookFor: 'Security vulnerabilities, breaches and urgent patches; not general security advice.' },
  { label: 'Price drops', name: 'Price drops', lookFor: 'Price cuts, sales and deals on things I might buy.' },
  { label: 'Job posts', name: 'Jobs', lookFor: 'New job openings that match my skills; not recruiter ads.' },
  { label: 'Outages and incidents', name: 'Outages', lookFor: 'Service outages, incidents and their resolution.' },
  { label: 'Anything new', name: 'Everything new', lookFor: '' },
]
export const FIRST_CHECK_LABEL = 'First check'
export const whyWords = (why: string): string => `Why: ${why}`
export const lookingForWords = (what: string): string => `Looking for: ${what}`
export const MAX_SOURCES = 10
export const ADD_SOURCE_LABEL = 'Add a source'
export const NEW_SOURCE_WORDS = 'not filled in yet'
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

export const deleteWatchTitle = 'Stop following this watch?'
export const deleteWatchMessage = (name: string): string => `"${name}" and what it found are removed from the feed.`

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

/** What a watch's last run says, in plain words. */
export const WATCH_STATUS_TAG: Record<WatchStatus, { state: StatusState; label: string }> = {
  baseline: { state: 'info', label: 'Recorded what is there' },
  nothing_new: { state: 'neutral', label: 'Nothing new' },
  in_feed: { state: 'success', label: 'In the feed' },
  sent: { state: 'success', label: 'Sent' },
  send_failed: { state: 'error', label: 'Send failed' },
  no_channel: { state: 'warning', label: 'No channel' },
  analysis_failed: { state: 'warning', label: 'Could not compare' },
  failing: { state: 'error', label: 'Source failing' },
}

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

