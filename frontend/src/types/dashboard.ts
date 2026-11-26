import type { Component } from 'vue'
import type { RouteLocationRaw } from 'vue-router'

// The dashboard: boards (tabs) of cards. The server keeps the layout (which card, in what order, how large); what a card shows
// is read by the card itself from its own stack. Cols is a width on a grid of twelve columns.

export type DashboardCols = 3 | 4 | 6 | 8 | 12
export type DashboardRows = 1 | 2

/** Where a card's facts come from; shown as the chip in its header and used to group the catalogue. */
export type DashboardSource = 'host' | 'assistant' | 'automations' | 'reminders' | 'network' | 'radio' | 'agents' | 'newsroom'

/** One card on a board, as the server stores it. */
export interface DashboardPlacement {
  id: string
  cols: DashboardCols
  rows: DashboardRows
}

export interface DashboardBoard {
  id: string
  name: string
  cards: DashboardPlacement[]
}

export interface DashboardLayout {
  boards: DashboardBoard[]
}

/** What the card catalogue knows about a card: where it comes from, its title, its default size and where its arrow goes. */
export interface DashboardCardDef {
  id: string
  source: DashboardSource
  title: string
  component: Component
  cols: DashboardCols
  rows: DashboardRows
  /** The control or settings page that acts on what the card shows; the card's arrow opens it. */
  control: () => RouteLocationRaw
}

/** A stack whose shipped boards are shown as one tab with a row of sub-tabs under it, not one tab each. */
export type DashboardGroup = 'network' | 'newsroom'

/** A shipped board that is one page to read rather than a grid of cards (ADR-0089): the Newsroom's brief and its feed. */
export type DashboardBoardView = 'newsroom-brief' | 'newsroom-feed'

/** How a board view is drawn: its component, and whether it takes the whole width (a page to read) or the usual page width (lists). */
export interface DashboardBoardViewDef {
  component: Component
  wide: boolean
}

/** A board that ships ready-made: a name and the cards it starts with. */
export interface DashboardBoardTemplate {
  id: string
  name: string
  /** Boards that share a group are one tab in the strip; the board's name is then its sub-tab. */
  group?: DashboardGroup
  /** Boards a stack ships carry the stack's source, so the catalogue can say where they come from. */
  source: DashboardSource | null
  /** A board that shows one view in place of cards: it has no cards to add, move or reset, and its view fills the board. */
  view?: DashboardBoardView
  cards: readonly DashboardPlacement[]
}

/** One sub-tab of a group: a board the group ships, whether or not the owner's layout has it yet (opening it adds it). */
export interface DashboardSubTab {
  id: string
  label: string
  present: boolean
}

/** One tab of the strip: a single board, or a group of boards with its sub-tabs. `id` is the board that opens when the tab is chosen. */
export interface DashboardTab {
  id: string
  label: string
  group?: DashboardGroup
  subTabs: DashboardSubTab[]
}

/** How much room a card has, by its own width: small shows a headline, medium adds a trend or a short list, large shows the detail. */
export type DashboardCardSize = 'xs' | 'sm' | 'lg'

/** The lines on stack pages that point to the dashboard: a stack's board, or one card of it. */
export type BoardHintKey = 'network' | 'radio' | 'agents' | 'newsroom'

/** The layout choices that re-flow the cards of one board. */
export type DashboardPreset = 'default' | 'compact' | 'two-columns' | 'one-column'
