import type { BriefReading, BriefStarter, BriefWriteStep, FollowMode, SourceGuide, SourceKind, SourceKindInfo, WatchStatus } from '../types/newsroom'
import type { ChoiceOption, StatusState } from '../types/ui'

// The words of the Newsroom screens.

export const NEWSROOM_TITLE = 'Newsroom'
export const NEWSROOM_SUBTITLE = 'The news you choose, written up once or twice a day, and alerts the moment something you watch changes.'
export const NEWSROOM_TAB_LABELS = { brief: 'Brief', alerts: 'Alerts', following: 'Following', settings: 'Settings' } as const
/** The two kinds of thing you follow, on the Following tab: topics that go in the brief, and watches that alert you. */
export const FOLLOWING_KIND_LABELS = { brief: 'In your brief', alerts: 'Alerts' } as const
export const FOLLOWING_VIEW_LABEL = 'What you follow'
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
// What the model left out (kept 7 days, at most 50 per watch), shown beside the alerts so a wrong call can be put back.
export const ALERTS_VIEW_LABEL = 'Show'
export const LEFT_OUT_VIEW_LABEL = 'Left out by the model'
export const leftOutViewWords = (count: number): string => (count > 0 ? `${LEFT_OUT_VIEW_LABEL} · ${count}` : LEFT_OUT_VIEW_LABEL)
// The line above the alerts says only what needs a look: a watch that cannot read its sources, send, or compare. Nothing when all is well.
export const ALERT_PROBLEM_WORDS = { failing: 'source failing', send_failed: 'send failed', no_channel: 'no channel', analysis_failed: 'could not compare' } as const
export const sourcesDownWords = (n: number): string => (n === 1 ? '1 source not answering' : `${n} sources not answering`)
export const ALERT_PROBLEMS_LINK = 'Open in Following'
// Always on the Alerts tab: the watches and their sources live on Following, behind its "Alerts" switch, so the way there is said, not known.
export const ALERTS_SETUP_LINK = 'Set up alerts'
// Every setup view links back to where its result is read, and the brief to where it is set up: one click each way.
export const FOLLOWING_READ_LINKS = { brief: 'Read the brief', alerts: 'See alerts' } as const
export const BRIEF_SETUP_LINK = 'Set up the brief'
export const leftOutWords = (why: string): string => `Left out: ${why}`
export const KEEP_LABEL = 'Should have matched'
export const KEPT_TOAST = 'Put in Alerts. The watch will learn from it.'
// "Not for me" on an alert: it leaves Alerts for "Left out by the model" (where "Should have matched" brings it back) and becomes an example the watch's
// model sees from its next check.
export const NOT_FOR_ME_LABEL = 'Not for me'
export const notForMeAria = (headline: string): string => `${NOT_FOR_ME_LABEL}: ${headline}`
export const NOT_FOR_ME_TOAST = 'Taken out of Alerts. The watch will learn from it.'
// The watch form's list of posts the owner judged (Watch.examples).
export const EXAMPLES_LABEL = 'What this watch learned from you'
export const EXAMPLES_HINT = 'Posts you marked with "Should have matched" or "Not for me". The model sees the newest 10 of each when it checks. Remove one you marked by mistake.'
export const EXAMPLES_KIND_LABELS = { want: 'Wanted', skip: 'Not for me' } as const
export const removeExampleLabel = (title: string): string => `Remove ${title}`
export const LEFT_OUT_EMPTY = 'Nothing was left out in the last 7 days.'
export const LEFT_OUT_HINT = 'Posts the model judged not to be what your watch looks for, with its reason. Kept for 7 days.'
export const ALERTS_UNREAD_POLL_MS = 30000
export const unreadAlertsWords = (count: number): string => (count === 1 ? '1 unread alert' : `${count} unread alerts`)
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
/** When a brief already exists the button makes another: it says so, since the page writes one by itself at the ready time. */
export const MAKE_ANOTHER_LABEL = 'Write a new one now'
export const TOP_STORIES_LABEL = 'Top stories'
export const SECTION_EMPTY = 'Nothing new here since the last edition.'
export const SECTION_NEVER_READ = 'Nothing has been read for this topic yet. Press "Read now" on the topic under Following, or wait for the next collection.'
export const UPDATE_LABEL = 'Update'
export const HEADLINE_ONLY_LABEL = 'Headline only'
export const BRIEF_EDITION_LABEL = 'Edition'
export const briefMadeWords = (when: string, calls: number): string => `Written ${when} · ${calls} model ${calls === 1 ? 'call' : 'calls'}`
export const briefSentWords = (clock: string): string => `Summary sent to Telegram at ${clock}`
export const BRIEF_SEND_PROBLEM: Record<'refused' | 'failed' | 'gave_up', string> = {
  refused: 'Telegram did not take the summary. Check that Telegram is switched on as a connector.',
  failed: 'The Telegram summary could not be sent just now. It is tried again for the next few hours.',
  gave_up: 'The Telegram summary could not be sent, and the time to try has passed.',
}
export const BRIEF_SEND_FIX = 'Open Connectors'
export const briefReportedWords = (outlets: string[]): string => `Reported by ${outlets.join(', ')}`
export const briefAsOfWords = (when: string): string => `Collected ${when}`
export const BRIEF_SECTION_SHOWN = 3
export const SECTION_IN_TOP = 'This topic\'s stories are in the top stories above.'
export const SECTION_HEADLINES_ONLY = 'These sources give headlines only, so there is no summary.'
export const showMoreWords = (n: number): string => `Show ${n} more`
export const SHOW_FEWER_LABEL = 'Show fewer'
export const LEAD_STORY_LABEL = 'Top story'
/** The top story's label on a card: "Top story", then its section when it has one. */
export const leadStoryWords = (section?: string): string => (section ? `${LEAD_STORY_LABEL} · ${section}` : LEAD_STORY_LABEL)
const LEAD_OUTLETS_SHOWN = 3
/** The outlets of a card's top story by name, three at most and how many more: "BBC · Reuters · FT +1". */
export const leadOutletsWords = (outlets: string[]): string =>
  outlets.slice(0, LEAD_OUTLETS_SHOWN).join(' · ') + (outlets.length > LEAD_OUTLETS_SHOWN ? ` +${outlets.length - LEAD_OUTLETS_SHOWN}` : '')
export const briefStoryCountWords = (n: number): string => `${n} ${n === 1 ? 'story' : 'stories'}`
/** A section's count: what the model wrote for it, and how many of those the top stories already show. */
export const briefSectionCountWords = (total: number, inTop: number): string => (inTop > 0 ? `${briefStoryCountWords(total)} · ${inTop} in ${TOP_STORIES_LABEL}` : briefStoryCountWords(total))
export const BRIEF_PROBLEMS_LABEL = 'Could not read'
/** What to do about a source that could not be read: the brief goes on with what it collected; the fix is on the topic, under Following. */
export const BRIEF_PROBLEMS_FIX = 'The brief used what it had already collected. Check the address on the topic, or remove the source.'
export const BRIEF_PROBLEMS_ACTION = 'Open Following'
export const BRIEF_EARLIER_LABEL = 'Earlier briefs'
// Deleting written briefs: the one on screen, or the older ones of the same brief.
export const BRIEF_DELETE_LABEL = 'Delete briefs'
export const BRIEF_DELETE_THIS_LABEL = 'Delete this brief'
// No count: the page lists only the newest briefs, so it cannot know how many older ones the server keeps. The toast says how many went.
export const briefDeleteOlderWords = (name: string): string => `Delete every older ${name} brief`
export const BRIEF_DELETE_TITLE = 'Delete briefs?'
export const briefDeleteMessage = (older: boolean, name: string): string =>
  older ? `This deletes every ${name} brief older than the one on screen, for good. The brief on screen stays.` : `This deletes the ${name} brief on screen for good. It is not written again.`
export const briefDeletedWords = (count: number): string => (count === 1 ? '1 brief deleted' : `${count} briefs deleted`)
export const briefEarlierOption = (when: string, latest: boolean): string => (latest ? `${when} (latest)` : when)
export const briefOutletsWords = (n: number): string => `read from ${n} ${n === 1 ? 'outlet' : 'outlets'}`

// Briefs setup.
export const TOPICS_TITLE = 'Topics'
export const EDITIONS_TITLE = 'Editions'
export const NEW_TOPIC_LABEL = 'Add a topic'
export const NEW_EDITION_LABEL = 'Add an edition'
export const NO_EDITIONS_SETUP_BODY = 'An edition is one brief: the news sections you pick, what your watches found and your home, written at a time you set.'
export const NEED_TOPIC_FIRST = 'Add a topic first to include news.'
export const TOPIC_NAME_LABEL = 'Topic name'
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
export const TOPIC_NAME_HINT = 'One subject, such as Greece. Left empty, the first search names it.'
export const TOPIC_CATEGORY_LABEL = 'Section'
export const TOPIC_CATEGORY_HINT = 'The heading this topic appears under in your brief. Topics with the same section are listed together.'
/** The sections offered first: the built-in source list's groups, so a pick lines up with the sources under it. */
export const BRIEF_CATEGORIES = ['News', 'Technology', 'Business and finance', 'Science and health', 'Sport'] as const
export const TOPIC_SEARCHES_LABEL = 'What to search for'
export const TOPIC_SEARCHES_HINT = 'Words you would type in a news search, such as Greece. Press Enter after each. Several searches are merged into this one topic.'
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
export const MAX_EDITION_TOPICS = 12
export const EDITION_NAME_HINT = 'Such as Morning or Evening.'
export const EDITION_READY_LABEL = 'Ready at'
export const EDITION_DAYS_LABEL = 'On these days'
export const EDITION_TOPICS_LABEL = 'What goes in it?'
export const EDITION_WATCHES_LABEL = 'What my watches found'
export const EDITION_HOME_LABEL = 'My home: network and radio'
export const HOME_BLOCK_LABEL = 'Home'
export const HOME_COUNTS_ONLY = 'Counts only. Names and addresses stay on the Network and Radio pages.'
export const HOME_QUIET_LABEL = 'All quiet'
export const BRIEF_ATTENTION_LABEL = 'Needs attention'
export const HOME_OPEN_LABEL = 'Show the Home facts'
export const HOME_FOLD_LABEL = 'Fold the Home facts'
/** The header of the Home block: how many facts are serious and how many are worth a look. */
export const homeStateWords = (critical: number, check: number): string =>
  [critical > 0 ? `${critical} critical` : '', check > 0 ? `${check} to check` : ''].filter(Boolean).join(' · ') || HOME_QUIET_LABEL
export const WATCHES_BLOCK_LABEL = 'Alerts'
export const WATCHES_SINCE_LAST = 'since the last edition'
export const watchNewWords = (n: number): string => `${n} new`
export const WATCH_SOURCE_PROBLEM = 'Source not read'
export const EDITION_DAY_LABEL = 'My day: reminders'
export const EDITION_DAY_HINT = 'Your own reminders for today, or for tomorrow in a brief that is ready from noon. Shown only to you, here.'
export const EDITION_SEND_LABEL = 'Send a short summary to Telegram'
export const EDITION_SEND_HINT = 'The top stories and the home counts, written to fit a phone. Never your reminders, device names or addresses. Needs Telegram set up as a connector; it carries a link only when Telegram is reached over https.'
export const DAY_BLOCK_LABEL = 'Your day'
export const dayHeading = (tomorrow: boolean): string => (tomorrow ? 'Tomorrow' : 'Today')
export const DAY_NONE = 'Nothing set for the day.'
export const DAY_UNAVAILABLE = 'Reminders is off or could not be read, so nothing is said about your day.'
export const DAY_REPEATS = 'repeats'
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
// The topic form asks for one thing, what to follow; the rest sits under Fine-tune. The starters are ready-made topics (name, section, searches and
// well-known sites by their name in the built-in list), offered as suggestions in the form and as a gallery on an empty page.
export const STARTERS_TITLE = 'Pick topics to start with'
export const STARTERS_HINT = 'Each comes with its searches and websites. Change anything afterwards from its card.'
export const STARTERS_ALL_LABEL = 'All'
export const STARTERS_OWN_LABEL = 'Make my own'
export const startersAddWords = (n: number): string => `Add ${n} ${n === 1 ? 'topic' : 'topics'}`
export const BRIEF_STARTERS: readonly BriefStarter[] = [
  { name: 'AI and chips', section: 'Technology', queries: ['AI chips', 'Nvidia'], sites: ['Ars Technica', 'The Verge'] },
  { name: 'World headlines', section: 'News', queries: [], sites: ['BBC World', 'The Guardian World', 'Al Jazeera'] },
  { name: 'Markets', section: 'Business and finance', queries: ['stock markets'], sites: ['CNBC Top News', 'Financial Times', 'MarketWatch Top Stories'] },
  { name: 'Space', section: 'Science and health', queries: ['space', 'NASA'], sites: ['NASA', 'BBC Science'] },
  { name: 'Football', section: 'Sport', queries: ['Premier League'], sites: ['BBC Sport'] },
  { name: 'Cybersecurity', section: 'Technology', queries: ['cybersecurity', 'ransomware'], sites: ['Krebs on Security', 'The Hacker News'] },
  { name: 'UK news', section: 'News', queries: [], sites: ['BBC UK', 'The Guardian UK', 'Sky News'] },
  { name: 'Crypto', section: 'Business and finance', queries: ['Bitcoin'], sites: ['CoinDesk'] },
  { name: 'Ukraine', section: 'News', queries: ['Ukraine'], sites: ['BBC World'] },
  { name: 'Health', section: 'Science and health', queries: ['health'], sites: ['BBC Health', 'WHO news'] },
  { name: 'Formula 1', section: 'Sport', queries: ['Formula 1'], sites: ['BBC Sport'] },
  { name: 'Tech headlines', section: 'Technology', queries: [], sites: ['The Verge', 'TechCrunch', 'Ars Technica'] },
]
export const TOPIC_NEVER_READ = 'Not read yet'
export const topicFollowsWords = (n: number): string => `+${n} ${n === 1 ? 'website' : 'websites'}`
export const MODEL_LIMIT_TITLE = 'Daily AI limit'
export const MODEL_LIMIT_HINT = (fallback: number, max: number): string => `A safety cap on how many times a day the Newsroom may ask the AI — watches and briefs share it. It does not set how often news is gathered. Empty uses ${fallback}; the most is ${max}.`
export const NO_ISSUE_HEADLINE = 'Nothing written yet'

// Topics by section, and the edition form's "What goes in it?"
export const TOPICS_INTRO = 'A section is a heading in the brief. A topic is one thing you follow, found by searching the news for its words.'
export const NEW_SECTION_LABEL = 'New section'
export const SECTION_NAME_LABEL = 'Section name'
export const SECTION_NAME_PLACEHOLDER = 'Section name, such as Sport'
export const ADD_SECTION_LABEL = 'Add section'
export const ADD_TOPIC_LABEL = 'Add topic'
export const addTopicPlaceholder = (section: string): string => `Add a topic to ${section}, such as Greece`
export const addTopicLabel = (section: string): string => `Add a topic to ${section}`
export const topicCountWords = (n: number): string => `${n} ${n === 1 ? 'topic' : 'topics'}`
export const NO_SECTION_LABEL = 'No section'
export const NO_SECTION_HINT = 'Each is read under its own name. Edit one to give it a section.'
export const TOPIC_SEARCHES_ROW = 'Searches'
export const TOPIC_READS_ROW = 'Reads'
export const THIN_TOPIC_HINT = 'One word brings a lot. Try a longer phrase, or add a site.'
export const EDITION_INCLUDE_HINT = 'Pick at least one. A brief can be only your home or your day.'
export const EDITION_NEWS_LABEL = 'News'
export const EDITION_NEWS_HINT = 'The sections you pick, each with its topics. A topic added to a section you picked joins the brief.'
export const EDITION_WATCHES_HINT = 'New items from the feeds you follow, since the last brief.'
export const EDITION_SECTIONS_LABEL = 'Sections in this brief'
export const removeSectionLabel = (name: string): string => `Remove ${name} from this brief`
export const addSectionLabel = (name: string): string => `Add ${name} to this brief`
export const EDITION_TOO_MANY = `A brief reads at most ${MAX_EDITION_TOPICS} topics. Take a section out, or delete topics you do not need.`
export const EDITION_NEEDS_SOMETHING = 'Pick at least one thing for the brief.'

// Unread briefs: the dot on the Briefs tab and the dashboard's Newsroom tab, and the dashboard's Brief card.
export const NEW_BRIEF_LABEL = 'New'
export const BRIEF_UNREAD_POLL_MS = 30000
export const unreadBriefWords = (count: number): string => (count === 1 ? '1 new brief' : `${count} new briefs`)
export const CRITICAL_HOME_DOT = 'Something in the latest brief needs a look'
export const BRIEF_CARD_LOAD_FAILED = 'The brief could not be read.'
export const BRIEF_CARD_RELOAD = 'It tries again by itself every minute.'
export const BRIEF_CARD_NONE = 'No brief yet. A brief is the news you choose, written at a time you set.'
export const BRIEF_CARD_SET_UP = 'Set up a brief'
export const BRIEF_CARD_WAITING = (ready: string): string => `Not written yet. It is ready at ${ready}.`
export const BRIEF_CARD_OPEN = 'Read the whole brief'
export const BRIEF_CARD_PICK_LABEL = 'Brief'
export const TODAY_CARD_READ = 'Read the brief'
export const BRIEF_CHIPS_LABEL = 'Needs a look elsewhere'
export const briefFailedRunsWords = (n: number): string => (n === 1 ? '1 automation run failed in the last day' : `${n} automation runs failed in the last day`)
export const briefBlockedWords = (n: number): string => (n === 1 ? '1 alert could not be sent' : `${n} alerts could not be sent`)
export const BRIEF_CHIPS_WINDOW_MS = 24 * 60 * 60 * 1000
export const BRIEF_CARD_EMPTY_ISSUE = 'This brief has no stories.'
export const briefCardCountWords = (stories: number, sections: number): string =>
  `${stories} ${stories === 1 ? 'story' : 'stories'}${sections > 1 ? ` in ${sections} sections` : ''}`
export const briefCardLineWords = (name: string, stories: number): string => `${name} · ${stories} ${stories === 1 ? 'story' : 'stories'}`

// A write by hand: the page follows it on the server until it ends.
export const BRIEF_WRITE_POLL_MS = 2000
export const writeStepWords = (s: BriefWriteStep): string => {
  if (s.stage === 'reading') return s.total ? `Reading the sources (${Math.min(s.done + 1, s.total)} of ${s.total})…` : 'Reading the sources…'
  if (s.section) return `Writing ${s.section} (${s.done + 1} of ${s.total})…`
  return 'Choosing the top stories…'
}

// An edition's reading settings, under Fine-tune in the edition form. The defaults match the server's (brief/compose.go).
export const DEFAULT_READING: BriefReading = { stories: 5, top: 4, repeatHours: 24, ageHours: 48 }
export const READING_STORIES_OPTIONS = [3, 5, 7, 10] as const
export const READING_TOP_OPTIONS = [3, 4, 5, 6, 8] as const
export const READING_REPEAT_HOURS = [12, 24, 48, 72, 168] as const
export const READING_AGE_HOURS = [12, 24, 48, 72] as const
export const EDITION_FINE_TUNE_LABEL = 'Fine-tune'
export const READING_STORIES_LABEL = 'Stories in each section'
export const READING_TOP_LABEL = 'Top stories'
export const READING_REPEAT_LABEL = 'Do not repeat a story for'
export const READING_AGE_LABEL = 'Only stories newer than'
export const READING_HINT = 'A story shown before comes back sooner when more outlets report it.'
export const daysOrHoursWords = (h: number): string => (h % 24 === 0 ? `${h / 24} ${h === 24 ? 'day' : 'days'}` : `${h} hours`)

// The brief as a page to read.
export const BRIEF_INDEX_LABEL = 'In this brief'
export const BRIEF_BESIDE_LABEL = 'Beside the news'
export const BRIEF_END_WORDS = 'You are up to date.'
export const BRIEF_PRINT_LABEL = 'Print'
export const BRIEF_COPY_LABEL = 'Copy as text'
export const BRIEF_COPIED = 'The brief is copied as text'
export const BRIEF_COPY_FAILED = 'The brief could not be copied. Your browser did not allow it.'
export const coverageWords = (n: number): string => `Reported by ${n} ${n === 1 ? 'outlet' : 'outlets'}`
export const COVERAGE_TICKS = 8
export const BRIEF_ATTENTION_LOOK = 'Look'

// Following something from one form, a story's own words, and feedback on a story.
export const FOLLOW_WORDS_MAX = 6
export const MAX_TOPIC_QUERIES = 10
export const MAX_TOPIC_IGNORE = 20
export const STOP_WORDS: ReadonlySet<string> = new Set(['the', 'and', 'for', 'with', 'from', 'that', 'this', 'are', 'was', 'were', 'has', 'have', 'had', 'its', 'his', 'her', 'their', 'into', 'over', 'after', 'about', 'says', 'said', 'will', 'would', 'could', 'can', 'not', 'but', 'out', 'new', 'how', 'why', 'what', 'who', 'when', 'than', 'more', 'amid', 'as', 'all', 'you', 'your', 'our', 'his', 'she', 'him', 'they', 'them', 'off', 'up', 'down', 'just', 'now', 'get', 'gets', 'also', 'one', 'two', 'per', 'via', 'report', 'reports'])
export const FOLLOW_LABEL = 'Follow'
export const FOLLOW_TITLE = 'Follow something'
export const FOLLOW_WHAT_LABEL = 'What do you want to follow?'
export const FOLLOW_WHAT_PLACEHOLDER = 'For example: open-weights model releases'
export const FOLLOW_HOW_LABEL = 'How do you want to hear about it?'
export const FOLLOW_MODES: Readonly<Record<FollowMode, { label: string; hint: string }>> = {
  brief: { label: 'In my brief', hint: 'Gathered through the day and written up in your brief, in the section you pick.' },
  alerts: { label: 'Alert me at once', hint: 'Checked every hour. Anything new goes to Alerts, and to a connector if you choose one.' },
  both: { label: 'Both', hint: 'Written up in your brief, and an alert as soon as something new appears.' },
}
export const FOLLOW_SECTION_LABEL = 'Section of the brief'
export const FOLLOW_SECTION_HINT = 'A section you have, or a new one. Empty uses the words above.'
export const FOLLOW_FEED_LABEL = 'Also read this feed (optional)'
export const FOLLOW_LOOK_FOR_LABEL = 'Only items about (optional)'
export const FOLLOW_LOOK_FOR_PLACEHOLDER = 'For example: a new model, not a benchmark'
export const FOLLOW_CHANNEL_LABEL = 'Also tell me on'
export const FOLLOW_PREVIEW_LABEL = 'Preview'
export const FOLLOW_PREVIEW_HEADING = 'What a news search finds now'
export const FOLLOW_PREVIEW_NONE = 'A news search finds nothing for these words right now. It may still find something later.'
export const FOLLOW_SAVE_LABEL = 'Follow'
export const followDoneWords = (mode: FollowMode, joined: number): string =>
  mode === 'alerts' ? 'Following: alerts are on' : joined > 0 ? `Following: in ${joined === 1 ? 'your brief' : `${joined} briefs`}${mode === 'both' ? ' and in Alerts' : ''}` : `Following: the topic is added${mode === 'both' ? ' and alerts are on' : ''}. Add its section to a brief to read it there.`
export const STORY_MORE_LABEL = 'More like this'
export const STORY_LESS_LABEL = 'Less like this'
export const STORY_FOLLOW_LABEL = 'Follow this story'
export const STORY_TUNE_LABEL = 'Tune this story'
export const storyMoreHint = (topic: string): string => `Pick the words: they become one more search of ${topic}.`
export const storyLessHint = (topic: string): string => `Pick the words: a story that mentions one of them is skipped in ${topic} from now on.`
export const storyTuneApply = (more: boolean): string => (more ? 'Add the search' : 'Skip these words')
export const STORY_TUNE_FULL = 'That topic already has as many searches or skip words as it can hold, or these are already there. Change it under Following.'
export const storyTunedWords = (more: boolean, topic: string): string => (more ? `${topic} searches for that too from the next collection` : `${topic} skips those words from the next collection`)
export const OPML_IMPORT_LABEL = 'Import a list (OPML)'
export const OPML_EXPORT_LABEL = 'Export my sources (OPML)'
export const OPML_NONE = 'The file holds no feed addresses this list can use.'
export const opmlImportedWords = (added: number, skipped: number): string => `${added} added${skipped ? `, ${skipped} already in a list or refused` : ''}`
export const OPML_FILE_NAME = 'my-sources.opml'
