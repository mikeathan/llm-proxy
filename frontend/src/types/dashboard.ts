import type { Component } from 'vue'
import type { RouteLocationRaw } from 'vue-router'

// The dashboard: boards (tabs) of cards. The server keeps the layout (which card, in what order, how large); what a card shows
// is read by the card itself from its own stack. Cols is a width on a grid of twelve columns.

export type DashboardCols = 3 | 4 | 6 | 8 | 12
export type DashboardRows = 1 | 2

/** Where a card's facts come from; shown as the chip in its header and used to group the catalogue. */
export type DashboardSource = 'host' | 'assistant' | 'automations' | 'reminders' | 'network' | 'radio' | 'agents'

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

/** A board that ships ready-made: a name and the cards it starts with. */
export interface DashboardBoardTemplate {
  id: string
  name: string
  /** Boards a stack ships carry the stack's source, so the catalogue can say where they come from. */
  source: DashboardSource | null
  cards: readonly DashboardPlacement[]
}

/** How much room a card has, by its own width: small shows a headline, medium adds a trend or a short list, large shows the detail. */
export type DashboardCardSize = 'xs' | 'sm' | 'lg'

/** The lines on stack pages that point to the dashboard: a stack's board, or one card of it. */
export type BoardHintKey = 'network' | 'radio'

/** The layout choices that re-flow the cards of one board. */
export type DashboardPreset = 'default' | 'compact' | 'two-columns' | 'one-column'
