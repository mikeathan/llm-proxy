import type { BoardHintKey, DashboardBoardTemplate, DashboardGroup, DashboardCardSize, DashboardCols, DashboardPreset, DashboardRows, DashboardSource } from '../types/dashboard'
import type { ChoiceOption } from '../types/ui'

// The dashboard's words and shipped layouts. The limits match core/dashboard on the server, which refuses more.

import type { InjectionKey, Ref } from 'vue'

/** Provided by a dashboard card to everything inside it: a Panel in a card draws only its content, because the card is the panel. */
export const IN_DASHBOARD_CARD: InjectionKey<boolean> = Symbol('in-dashboard-card')

/** Provided by a dashboard card to everything inside it: the size its own width gives it. Outside a card (or before it is measured) it is large. */
/** The room a card's list gets at each card size: the list scrolls inside it, so a smaller card shows fewer rows at a time and never hides the rest. */
export const CARD_LIST_SCROLL: Record<DashboardCardSize, string> = { xs: '', sm: 'max-h-72 overflow-y-auto [scrollbar-gutter:stable]', lg: 'max-h-[36rem] overflow-y-auto [scrollbar-gutter:stable]' }

export const DASHBOARD_CARD_SIZE: InjectionKey<Ref<DashboardCardSize>> = Symbol('dashboard-card-size')
/** Provided by a dashboard card to everything inside it: whether the card is on screen now. Polling inside a card pauses while it is not, and catches up when it returns. */
export const DASHBOARD_CARD_VISIBLE: InjectionKey<Ref<boolean>> = Symbol('dashboard-card-visible')
/** A card narrower than this is small; narrower than the next, medium; otherwise large (in pixels of the card's own width). */
export const SMALL_CARD_BELOW_PX = 300
export const LARGE_CARD_FROM_PX = 560

export const DASHBOARD_TITLE = 'Dashboard'
export const DASHBOARD_EYEBROW = 'Control center'
export const DASHBOARD_SUBTITLE = 'Your boards, made of cards from every part of the system'
export const HOME_BOARD_ID = 'home'
/** The Newsroom's two boards: the brief, and the feed of what the watches found. */
export const NEWSROOM_BRIEF_BOARD = 'newsroom'
export const NEWSROOM_FEED_BOARD = 'newsroom-feed'

export const MAX_BOARDS = 20
export const MAX_CARDS_PER_BOARD = 40
export const MAX_BOARD_NAME = 40
export const NEW_BOARD_NAME = 'New board'

export const SIZE_OPTIONS: readonly { cols: DashboardCols; label: string; name: string }[] = [
  { cols: 3, label: 'S', name: 'Small' },
  { cols: 4, label: 'M', name: 'Medium' },
  { cols: 6, label: 'M+', name: 'Medium wide' },
  { cols: 8, label: 'L', name: 'Large' },
  { cols: 12, label: 'Full', name: 'Full width' },
]
export const ROWS_OPTIONS: readonly { rows: DashboardRows; label: string; name: string }[] = [
  { rows: 1, label: '1×', name: 'One row tall' },
  { rows: 2, label: '2×', name: 'Two rows tall' },
]

export const SOURCE_LABELS: Record<DashboardSource, string> = {
  host: 'LLM and host',
  assistant: 'Assistant',
  automations: 'Automations',
  reminders: 'Reminders',
  network: 'Network',
  radio: 'Radio',
  agents: 'Agents',
  newsroom: 'Newsroom',
}
/** The short chip in a card's header. */
export const SOURCE_CHIPS: Record<DashboardSource, string> = {
  host: 'HOST',
  assistant: 'ASSISTANT',
  automations: 'AUTOMATIONS',
  reminders: 'REMINDERS',
  network: 'NETWORK',
  radio: 'RADIO',
  agents: 'AGENTS',
  newsroom: 'NEWSROOM',
}
/** The tab a group of boards has in the strip. */
export const GROUP_LABELS: Record<DashboardGroup, string> = { network: 'Network', newsroom: 'Newsroom' }
/** The name of the row of sub-tabs, for screen readers. */
export const SUB_TABS_LABEL = (group: string): string => `${group} views`
export const SOURCE_ORDER: readonly DashboardSource[] = ['host', 'assistant', 'automations', 'reminders', 'network', 'radio', 'agents', 'newsroom']

export const PRESET_OPTIONS: ChoiceOption[] = [
  { value: 'default', label: 'Default' },
  { value: 'compact', label: 'Compact tiles' },
  { value: 'two-columns', label: 'Two columns' },
  { value: 'one-column', label: 'One column' },
]
export const PRESET_COLS: Record<Exclude<DashboardPreset, 'default'>, DashboardCols> = { compact: 3, 'two-columns': 6, 'one-column': 12 }
export const PINNED_COLS: DashboardCols = 4

/**
 * The line on a stack's page that points to where its views went on the dashboard: the board, and (when a view is one card) the card, so the
 * dashboard can say so when the owner's saved board does not have it yet.
 */
export const BOARD_HINT: Record<BoardHintKey, { text: string; link: string; board: string }> = {
  network: { text: 'Devices, findings, audits, traffic and the timeline are on the dashboard.', link: 'Open the Network tab', board: 'network' },
  agents: { text: 'What the agents did, and where they fetched from, is on the dashboard.', link: 'Open the Agents board', board: 'agents' },
  radio: { text: 'The radar, your devices, your networks and what changed are on the dashboard.', link: 'Open the Radio board', board: 'radio' },
  newsroom: { text: 'The brief and the feed are read on the dashboard.', link: 'Open the Newsroom tab', board: NEWSROOM_BRIEF_BOARD },
}/** The Network sub-tab (board) each Network card is on, so a link to a card opens the board that holds it. */
export const NETWORK_CARD_BOARD: Readonly<Record<string, string>> = {
  'network-overview': 'network',
  'network-devices': 'network-devices',
  'network-traffic': 'network-traffic',
  'network-findings': 'network-findings',
  'network-audits': 'network-findings',
  'network-timeline': 'network-activity',
}
export const CARD_MISSING = (title: string): string => `${title} is not on this board.`
export const CARD_MISSING_ADD = 'Add it'
export const NEW_CARDS_TITLE = (titles: string[]): string => `New on this board: ${titles.join(', ')}.`
export const NEW_CARDS_ADD = 'Add them'
export const NEW_CARDS_DISMISS = 'Not now'
export const ARRANGE_LABEL = 'Edit'
export const EDITING_BOARD_LABEL = (board: string): string => `Editing ${board}`
export const ARRANGE_DONE_LABEL = 'Done'
export const ADD_CARD_LABEL = 'Add card'
export const ADD_BOARD_LABEL = 'Add board'
export const RESET_ALL_LABEL = 'Put every board back as shipped'
export const RESET_ALL_CONFIRM = { title: 'Put every board back as shipped?', message: 'Your boards, the cards on them and their order go back to how they came: boards you added or renamed are removed.', confirmText: 'Put them back' } as const
export const RESET_BOARD_LABEL = 'Reset board'
export const PIN_LABEL = 'Pin to Home'
export const PINNED_LABEL = 'On Home'
export const REMOVE_CARD_LABEL = 'Remove card'
export const REMOVE_BOARD_LABEL = 'Remove board'
export const RENAME_BOARD_LABEL = 'Rename board'
export const EMPTY_BOARD_TITLE = 'This board is empty'
export const EMPTY_BOARD_BODY = 'Press Edit, then Add card, to show something here.'
export const LAYOUT_SAVE_FAILED = 'The layout could not be saved. It is back to what was saved.'
export const LAYOUT_LOAD_FAILED = 'The dashboard layout could not be read.'
export const ALL_CARDS_ADDED = 'Every card is already on this board.'
export const BOARD_ALREADY_ADDED = 'Added'

// The boards that ship ready-made. A stack's board is an ordinary board built from the same cards; the owner can remove, rename,
// re-arrange or reset it. Home is fixed and mixes cards from every source, but ships minimal: only a card no stack board already shows, so
// nothing is repeated; what else to bring from another board is the person's choice (Add card, or Pin to Home).
export const BOARD_TEMPLATES: readonly DashboardBoardTemplate[] = [
  {
    id: HOME_BOARD_ID,
    name: 'Home',
    source: null,
    cards: [
      { id: 'assistant-recent', cols: 6, rows: 1 },
      { id: 'newsroom-today', cols: 6, rows: 1 },
    ],
  },
  {
    id: 'host',
    name: 'LLM and host',
    source: 'host',
    cards: [
      { id: 'host-health', cols: 12, rows: 1 },
      { id: 'running-now', cols: 6, rows: 1 },
      { id: 'recent-runs', cols: 6, rows: 1 },
    ],
  },
  // Network is one tab with five sub-tabs, one topic each (ADR-0070). The first keeps the id `network`, which the stack's links use.
  { id: 'network', name: 'Overview', group: 'network', source: 'network', cards: [{ id: 'network-overview', cols: 12, rows: 1 }] },
  { id: 'network-devices', name: 'Devices', group: 'network', source: 'network', cards: [{ id: 'network-devices', cols: 12, rows: 1 }] },
  { id: 'network-traffic', name: 'Traffic', group: 'network', source: 'network', cards: [{ id: 'network-traffic', cols: 12, rows: 1 }] },
  {
    id: 'network-findings',
    name: 'Findings',
    group: 'network',
    source: 'network',
    cards: [
      { id: 'network-findings', cols: 12, rows: 1 },
      { id: 'network-audits', cols: 12, rows: 1 },
    ],
  },
  { id: 'network-activity', name: 'Activity', group: 'network', source: 'network', cards: [{ id: 'network-timeline', cols: 12, rows: 1 }] },
  {
    id: 'radio',
    name: 'Radio',
    source: 'radio',
    cards: [
      { id: 'radio-around', cols: 12, rows: 2 },
      { id: 'radio-devices', cols: 12, rows: 1 },
      { id: 'radio-sensors', cols: 6, rows: 1 },
      { id: 'radio-manage', cols: 6, rows: 1 },
      { id: 'radio-networks', cols: 6, rows: 1 },
      { id: 'radio-findings', cols: 12, rows: 1 },
    ],
  },
  // The Newsroom is read here (ADR-0089): one tab with two sub-tabs, each a page in place of cards. The first keeps the id `newsroom`, which the brief's links use.
  { id: NEWSROOM_BRIEF_BOARD, name: 'Brief', group: 'newsroom', source: 'newsroom', view: 'newsroom-brief', cards: [] },
  { id: NEWSROOM_FEED_BOARD, name: 'Feed', group: 'newsroom', source: 'newsroom', view: 'newsroom-feed', cards: [] },
  {
    id: 'agents',
    name: 'Agents',
    source: 'agents',
    cards: [{ id: 'agents-activity', cols: 12, rows: 2 }],
  },
  {
    id: 'automations',
    name: 'Automations',
    source: 'automations',
    cards: [
      { id: 'recent-runs', cols: 8, rows: 1 },
      { id: 'reminders', cols: 4, rows: 1 },
    ],
  },
]
