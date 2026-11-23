import type { SourceGuide, SourceKind, SourceKindInfo, WatchStatus } from '../types/newsroom'
import type { ChoiceOption, StatusState } from '../types/ui'

// The words of the Newsroom screens.

export const NEWSROOM_TITLE = 'Newsroom'
export const NEWSROOM_SUBTITLE = 'What is new about the things you care about. Set up a watch once, or ask the assistant to.'
export const NEWSROOM_TAB_LABELS = { feed: 'Feed', briefs: 'Briefs', watches: 'Watches', settings: 'Settings' } as const
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
export const FILTER_PERIOD_LABEL = 'Show a period'
export const FEED_FILTER_EMPTY = 'Nothing here for this filter. Try another watch or a longer period.'
// Below this many watches there is nothing to choose between, so the chip row is not shown.
export const MIN_WATCHES_FOR_CHIPS = 2
// The feed keeps 30 days, so "All" is the last 30 days; the values are calendar days in the reader's zone.
export const PERIOD_OPTIONS: { value: string; label: string }[] = [
  { value: '', label: 'All' },
  { value: '1', label: 'Today' },
  { value: '7', label: '7 days' },
  { value: '14', label: '14 days' },
]
export const MARK_READ_LABEL = 'Mark read'
export const MARK_ALL_READ_LABEL = 'Mark all read'
export const SELECT_LABEL = 'Select'
export const DONE_LABEL = 'Done'
export const SELECT_ALL_LABEL = 'Select all'
export const CLEAR_LABEL = 'Clear'
export const CLEAR_READ_LABEL = 'Clear read'
export const CLEAR_ALL_LABEL = 'Clear all'
export const SHOW_OLDER_LABEL = 'Show older days'
export const SHOW_DETAILS_LABEL = 'Show details'
export const HIDE_DETAILS_LABEL = 'Hide details'
export const selectedWords = (n: number): string => `${n} selected`
export const unreadWords = (n: number): string => `${n} unread`
export const clearTitle = 'Delete from the feed?'
export const clearMessage = (what: 'selected' | 'read' | 'all', n: number): string =>
  `${n === 1 ? '1 entry' : `${n} entries`}${what === 'selected' ? '' : what === 'read' ? ' you have read' : ''} will be deleted for good. They will not come back from the same source.`
export const clearedWords = (n: number): string => `Cleared ${n === 1 ? '1 entry' : `${n} entries`}`
export const CHIP_CHANGED = 'Changed'
export const ALSO_SEEN_LABEL = 'Also seen'
export const UNREAD_LABEL = 'Unread'

export const CARD_NO_WATCHES = 'You follow nothing yet. Ask the assistant to keep an eye on something.'
export const CARD_HEADLINES_EMPTY = 'Nothing new yet.'
export const CARD_FAILING_WORDS = (names: readonly string[]): string => `${names.join(', ')} cannot read a source.`
export const UNREAD_SOURCE_LABEL = 'Source not read'
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
export const SOURCE_REGION_LABEL = 'Region'
export const SOURCE_REGION_HINT = 'A language and a country, such as en-GB. Empty means en-US.'
export const RUN_NOW_LABEL = 'Check now'
export const RUNNING_LABEL = 'Checking…'
export const RUNNING_WORDS = 'Reading the sources and picking what matches. This can take a minute with a local model.'
export const originalWords = (title: string): string => `Original: ${title}`
export const PAUSED_LABEL = 'Paused'
export const FEED_ONLY_WORDS = 'Feed only'
export const sendsToWords = (channel: string): string => `Sends to ${channel}`
export const NEVER_CHECKED = 'Not checked yet'
export const lastCheckedWords = (relative: string): string => `Checked ${relative}`
export const updatesWords = (n: number): string => `${n} ${n === 1 ? 'update' : 'updates'}`
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
export const WATCH_MAX_AGE_LABEL = 'Only news from the last'
export const WATCH_MAX_AGE_HINT = 'Older posts are skipped, not announced; a post with no date counts as new. Widen it to look back further.'
export const MAX_AGE_OPTIONS: ChoiceOption[] = [
  { value: '24h', label: 'Day' },
  { value: '72h', label: '3 days' },
  { value: '168h', label: '7 days' },
  { value: '336h', label: '14 days' },
  { value: '720h', label: '30 days' },
]
export const DEFAULT_MAX_AGE = '72h'
export const postedWords = (when: string): string => `posted ${when}`

export const SOURCE_KIND_INFO: Record<SourceKind, SourceKindInfo> = {
  feed: { label: 'News feed or site (RSS, Atom, GitHub releases)', targetLabel: 'Address', hint: 'For example https://go.dev/blog/feed.atom' },
  hackernews: { label: 'Hacker News search', targetLabel: 'Search words', hint: 'Stories matching these words' },
  mastodon: { label: 'Mastodon tag', targetLabel: 'Tag', hint: 'The tag without the #' },
  newssearch: { label: 'News search (many publishers)', targetLabel: 'Search words', hint: 'Headlines from many outlets. Try inflation, or site:reuters.com when:1d' },
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
  region: 'A region is a language and a country such as en-GB or en-US.',
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
  newssearch: {
    name: 'News search',
    goodFor: 'The latest headlines about anything, from many outlets at once, including Reuters and AP, which have no feed of their own.',
    steps: [
      'Type the words you care about, such as inflation or Ukraine ceasefire.',
      'To read one outlet, type site:reuters.com. Add when:1d for the last day.',
      'Pick a region such as en-GB if you want that country\'s edition.',
    ],
    example: 'site:reuters.com when:1d',
    note: 'You get the headline, the outlet and the time, with no summary. The link opens a Google page that sends you on to the article. This is an unofficial service and can change.',
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

/** The look-up under each source of the watch form: see what a source would read before saving it. */
export const LOOKUP_LABEL = 'Look up'
export const LOOKUP_BUSY = 'Looking…'
export const LOOKUP_LATEST_LABEL = 'Latest'
export const LOOKUP_FAILED = 'The look-up could not be made. Try again in a moment.'
export const LOOKUP_PICK_LABEL = 'Pick one'
export const LOOKUP_NOTHING_YET = 'Nothing posted yet.'
export const lookupSavedAs = (value: string): string => `Will be saved as ${value}`
export const LOOKUP_HINTS: Record<SourceKind, string> = {
  feed: 'Paste a site address and the look-up finds its feed.',
  hackernews: 'Shows the stories this search finds right now.',
  mastodon: 'Shows the latest posts of this tag on this server.',
  newssearch: 'Shows the latest headlines this search finds.',
  page: 'Shows the title and the text that will be compared.',
  telegram: 'Checks that the channel has a public preview and shows its latest posts. A few look-ups a minute are allowed.',
}
export const trendingLabel = (host: string): string => `Trending on ${host}`
export const matchingLabel = (typed: string): string => `Tags matching "${typed}"`

// Briefs: editions of the news topics you choose, written once a day or so from what was collected.
export const BRIEFS_LOAD_FAILED = 'The briefs could not be read.'
export const BRIEFS_ACTION_FAILED = 'The change could not be made.'
export const NO_EDITIONS_TITLE = 'No editions yet'
export const NO_EDITIONS_BODY = 'An edition is a brief of the news topics you choose, ready at a time you set. Add a topic, then an edition that lists it.'
export const NO_ISSUE_TITLE = 'Not written yet'
export const NO_ISSUE_BODY = 'This edition is written shortly before its ready time. You can write it now from what has been collected.'
export const MAKE_NOW_LABEL = 'Write it now'
export const MAKING_LABEL = 'Writing…'
export const TOP_STORIES_LABEL = 'Top stories'
export const SECTION_EMPTY = 'Nothing new here since the last edition.'
export const SECTION_NEVER_READ = 'Nothing has been read for this topic yet. Press "Read now" on Set up, or wait for the next collection.'
export const UPDATE_LABEL = 'Update'
export const HEADLINE_ONLY_LABEL = 'Headline only'
export const BRIEF_EDITION_LABEL = 'Edition'
export const briefMadeWords = (when: string, calls: number): string => `Written ${when} · ${calls} model ${calls === 1 ? 'call' : 'calls'}`
export const briefReportedWords = (outlets: string[]): string => `Reported by ${outlets.join(', ')}`
export const briefAsOfWords = (when: string): string => `Collected ${when}`
export const BRIEF_SECTION_SHOWN = 3
export const SECTION_IN_TOP = 'This topic\'s stories are in the top stories above.'
export const SECTION_HEADLINES_ONLY = 'These sources give headlines only, so there is no summary.'
export const COLLAPSE_ALL_LABEL = 'Collapse all'
export const EXPAND_ALL_LABEL = 'Expand all'
export const showMoreWords = (n: number): string => `Show ${n} more`
export const SHOW_FEWER_LABEL = 'Show fewer'
export const LEAD_STORY_LABEL = 'Top story'
export const briefStoryCountWords = (n: number): string => `${n} ${n === 1 ? 'story' : 'stories'}`
/** A section's count: what the model wrote for it, and how many of those the top stories already show. */
export const briefSectionCountWords = (total: number, inTop: number): string => (inTop > 0 ? `${briefStoryCountWords(total)} · ${inTop} in ${TOP_STORIES_LABEL}` : briefStoryCountWords(total))
export const BRIEF_PROBLEMS_LABEL = 'Could not read'

// Briefs setup.
export const BRIEFS_TAB_LABELS = { read: 'Read', setup: 'Set up' } as const
export const BRIEFS_VIEW_LABEL = 'Briefs'
export const TOPICS_TITLE = 'Topics'
export const EDITIONS_TITLE = 'Editions'
export const NEW_TOPIC_LABEL = 'Add a topic'
export const NEW_EDITION_LABEL = 'Add an edition'
export const NO_TOPICS_BODY = 'A topic is a news area you choose, such as Technology, read from searches and feeds.'
export const NO_EDITIONS_SETUP_BODY = 'An edition lists some topics and is written at a time you set.'
export const NEED_TOPIC_FIRST = 'Add a topic first: an edition is made of topics.'
export const TOPIC_NAME_LABEL = 'Name'
export const TOPIC_NEEDS_SOMETHING = 'A topic needs at least one search or one feed.'
export const READ_NOW_LABEL = 'Read now'
export const EDITION_NAME_LABEL = 'Name'
export const DAY_OPTIONS: { value: string; label: string }[] = [
  { value: '1', label: 'Monday' }, { value: '2', label: 'Tuesday' }, { value: '3', label: 'Wednesday' }, { value: '4', label: 'Thursday' },
  { value: '5', label: 'Friday' }, { value: '6', label: 'Saturday' }, { value: '0', label: 'Sunday' },
]
export const EVERY_HOURS = [1, 2, 3, 4, 6, 12] as const
export const FOR_HOURS = [2, 4, 6, 12, 24] as const
export const DEFAULT_EVERY_HOURS = 2
export const DEFAULT_FOR_HOURS = 12
export const DEFAULT_READY_BY = '07:00'
export const ALL_DAYS = [0, 1, 2, 3, 4, 5, 6]
export const hoursWords = (n: number): string => `${n} ${n === 1 ? 'hour' : 'hours'}`
export const deleteTopicTitle = 'Delete this topic?'
export const deleteTopicMessage = (name: string): string => `"${name}" is removed. An edition that lists it must drop it first.`
export const deleteEditionTitle = 'Delete this edition?'
export const deleteEditionMessage = (name: string): string => `"${name}" is removed.`
export const DELETE_ALL_LABEL = 'Delete all briefs'
export const deleteAllTitle = 'Delete all briefs?'
export const DELETE_ALL_MESSAGE = 'This permanently removes every topic, every edition, every collected story and the sources you added. The built-in sources and your daily model-call limit stay. This cannot be undone.'
export const DELETE_ALL_DONE = 'All briefs deleted'
export const topicReadWords = (n: number): string => (n === 0 ? 'Read: nothing new.' : `Read: ${n} new ${n === 1 ? 'story' : 'stories'}.`)
export const editionSummaryWords = (ready: string, topics: number): string => `Ready by ${ready} · ${topics} ${topics === 1 ? 'topic' : 'topics'}`

export const MODEL_CALLS_LABEL = 'Model calls a day'
export const modelCallsProblem = (text: string, max: number): string => {
  const t = text.trim()
  if (t === '') return ''
  const n = Number(t)
  return Number.isInteger(n) && n >= 1 && n <= max ? '' : `Use a whole number from 1 to ${max}, or leave it empty.`
}

// Topic and edition forms
export const TOPIC_NAME_HINT = 'A short name for what this topic follows, such as AI chips.'
export const TOPIC_CATEGORY_LABEL = 'Section'
export const TOPIC_CATEGORY_HINT = 'Topics with the same section are read together under one heading in the brief. Pick one or type your own; left empty, this topic is a section of its own.'
/** The sections offered first: the built-in source list's groups, so a pick lines up with the sources under it. */
export const BRIEF_CATEGORIES = ['News', 'Technology', 'Business and finance', 'Science and health', 'Sport'] as const
export const TOPIC_SEARCHES_LABEL = 'What should it find?'
export const TOPIC_SEARCHES_HINT = 'Type a search and press Enter, as you would in a news search: AI chips, Ukraine, site:reuters.com for one publisher.'
export const TOPIC_SEARCHES_PLACEHOLDER = 'AI chips…'
export const TOPIC_FEEDS_LABEL = 'Follow a website (optional)'
export const TOPIC_FEEDS_HINT = 'Its newest stories are read alongside your searches. Start typing a site you know, or paste the link of its feed (RSS).'
export const TOPIC_FEEDS_PLACEHOLDER = 'BBC, Guardian, or https://example.com/feed.xml'
export const TOPIC_FEEDS_REJECTED = 'Pick a site from the list, or paste its feed link, which starts with https://.'
export const isFeedAddress = (v: string): boolean => /^https?:\/\/\S+$/i.test(v)
export const FEED_CHECKING = 'Checking…'
export const feedFoundWords = (title: string | undefined, n: number): string => `Reads fine${title ? `: ${title}` : ''} · ${n} recent ${n === 1 ? 'story' : 'stories'}`
export const FEED_NOT_A_FEED = 'This link could not be read as a feed.'
export const FEED_OFFERS = 'This page offers these feeds:'
export const FEED_SAVE_LABEL = 'Save to my sources'
export const FEED_CHECK_NOTE = 'Not in your lists. The check is a hint: the topic still saves.'
export const SOURCES_BROWSE_LABEL = 'Browse well-known sources'
export const SOURCES_BROWSE_HINT = 'Add a site to follow, or a publisher to search.'
export const SOURCE_ADD_LABEL = 'Add'
export const SOURCE_ADDED_LABEL = 'Added'
export const SOURCE_SEARCH_NOTE = 'searches this publisher'
export const MY_SOURCES_TITLE = 'My sources'
export const MY_SOURCES_BODY = 'Websites you add here are offered, with the well-known ones, when you follow a website in a topic.'
export const MY_SOURCES_EMPTY = 'You have not added any yet.'
export const MY_SOURCE_NAME_LABEL = 'Name'
export const MY_SOURCE_NAME_PLACEHOLDER = 'My favourite blog'
export const MY_SOURCE_LINK_LABEL = 'Feed link'
export const MY_SOURCE_LINK_PLACEHOLDER = 'https://example.com/feed.xml'
export const MY_SOURCE_ADD_LABEL = 'Add to my sources'
export const TOPIC_IGNORE_LABEL = 'Skip stories that mention (optional)'
export const TOPIC_IGNORE_HINT = 'Type a word and press Enter. A story with that word is left out.'
export const TOPIC_REGION_LABEL = 'News from'
export const REGION_DEFAULT_LABEL = 'Default'
export const REGION_OPTIONS: { value: string; label: string }[] = [
  { value: '', label: REGION_DEFAULT_LABEL }, { value: 'en-GB', label: 'United Kingdom (English)' }, { value: 'en-US', label: 'United States (English)' },
  { value: 'en-AU', label: 'Australia (English)' }, { value: 'en-CA', label: 'Canada (English)' }, { value: 'en-IE', label: 'Ireland (English)' },
  { value: 'de-DE', label: 'Germany (German)' }, { value: 'fr-FR', label: 'France (French)' }, { value: 'es-ES', label: 'Spain (Spanish)' },
]
export const EDITION_NAME_HINT = 'Such as Morning or Evening.'
export const EDITION_READY_LABEL = 'Ready at'
export const EDITION_DAYS_LABEL = 'On these days'
export const EDITION_TOPICS_LABEL = 'Which topics?'
export const EDITION_WATCHES_LABEL = 'Include what my watches found'
export const EDITION_HOME_LABEL = 'Include my network and radio'
export const HOME_BLOCK_LABEL = 'Home'
export const HOME_COUNTS_ONLY = 'Counts only. Names and addresses stay on the Network and Radio pages.'
export const WATCHES_BLOCK_LABEL = 'Your watches'
export const WATCHES_SINCE_LAST = 'since the last edition'
export const watchNewWords = (n: number): string => `${n} new`
export const WATCH_SOURCE_PROBLEM = 'Source not read'
export const EDITION_ON_LABEL = 'Write this edition automatically'
export const GATHER_BEFORE = 'Gather news every'
export const GATHER_START = 'starting'
export const GATHER_AFTER = 'before it is ready.'
export const gatherWords = (every: number, from: number): string => `Gathers every ${hoursWords(every)}, from ${hoursWords(from)} before.`
export const DAY_SHORT: Record<string, string> = { '1': 'Mon', '2': 'Tue', '3': 'Wed', '4': 'Thu', '5': 'Fri', '6': 'Sat', '0': 'Sun' }
export const DAY_ORDER = ['1', '2', '3', '4', '5', '6', '0'] as const
export const daysWords = (days: readonly number[]): string => {
  const set = new Set(days.map(String))
  if (set.size === 7) return 'Every day'
  if (set.size === 5 && ['1', '2', '3', '4', '5'].every((d) => set.has(d))) return 'Mon–Fri'
  return DAY_ORDER.filter((d) => set.has(d)).map((d) => DAY_SHORT[d]).join(', ')
}
export const editionCardWords = (ready: string, days: readonly number[]): string => `Ready at ${ready} · ${daysWords(days)}`
export const EDITION_OFF_LABEL = 'Paused'
export const EDITION_ON_BADGE = 'On'
export const topicLastWords = (when: string, n: number | undefined): string => `Last read ${when}${n === undefined ? '' : ` · ${n} new`}`
export const TOPIC_NEVER_READ = 'Not read yet'
export const topicFollowsWords = (n: number): string => `+${n} ${n === 1 ? 'website' : 'websites'}`
export const MODEL_LIMIT_TITLE = 'Daily AI limit'
export const MODEL_LIMIT_HINT = (fallback: number, max: number): string => `A safety cap on how many times a day the Newsroom may ask the AI — watches and briefs share it. It does not set how often news is gathered. Empty uses ${fallback}; the most is ${max}.`
export const READ_EDITION_READY = (ready: string, topics: number): string => `Ready at ${ready} · ${topics} ${topics === 1 ? 'topic' : 'topics'}`
export const NO_ISSUE_HEADLINE = 'Nothing written yet'
