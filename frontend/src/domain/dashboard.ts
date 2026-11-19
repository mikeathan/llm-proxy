import { BOARD_TEMPLATES, HOME_BOARD_ID, LARGE_CARD_FROM_PX, SMALL_CARD_BELOW_PX, MAX_BOARDS, MAX_BOARD_NAME, MAX_CARDS_PER_BOARD, NEW_BOARD_NAME, PINNED_COLS, PRESET_COLS } from '../constants/dashboard'
import { STACK_IDS } from '../types/stacks'
import type { DashboardBoard, DashboardBoardTemplate, DashboardCardDef, DashboardCardSize, DashboardCols, DashboardLayout, DashboardPlacement, DashboardPreset, DashboardRows, DashboardSource } from '../types/dashboard'

// The dashboard's layout rules, as pure functions: each takes a layout (or a board's cards) and returns a new one, so the
// composable that owns the state stays thin and every rule is testable without a screen.

/** The size a card has for its own width in pixels. Not yet measured (zero or not a number) counts as large, so nothing is hidden before layout. */
export function sizeForWidth(px: number): DashboardCardSize {
  if (!Number.isFinite(px) || px <= 0) return 'lg'
  if (px < SMALL_CARD_BELOW_PX) return 'xs'
  return px < LARGE_CARD_FROM_PX ? 'sm' : 'lg'
}

const clone = (cards: readonly DashboardPlacement[]): DashboardPlacement[] => cards.map((c) => ({ ...c }))

const boardFromTemplate = (t: DashboardBoardTemplate): DashboardBoard => ({ id: t.id, name: t.name, cards: clone(t.cards) })

export const templateOf = (boardId: string): DashboardBoardTemplate | undefined => BOARD_TEMPLATES.find((t) => t.id === boardId)

/** The boards a person starts with: Home and every board a stack ships. */
export function shippedLayout(): DashboardLayout {
  return { boards: BOARD_TEMPLATES.filter((t) => t.id === HOME_BOARD_ID || t.source !== null).map(boardFromTemplate) }
}

/**
 * What the screen shows for a stored layout. Nothing stored means the shipped boards. A card the screen has no definition for
 * (a stack removed since) is dropped; Home is always there, because it is the page people land on.
 */
export function reconcile(stored: DashboardLayout, knownCards: ReadonlySet<string>): DashboardLayout {
  if (!stored.boards.length) return pruned(shippedLayout(), knownCards)
  const boards = pruned(stored, knownCards).boards
  if (boards.some((b) => b.id === HOME_BOARD_ID)) return { boards }
  const home = templateOf(HOME_BOARD_ID)
  return { boards: [...(home ? [boardFromTemplate(home)] : []), ...boards] }
}

function pruned(layout: DashboardLayout, known: ReadonlySet<string>): DashboardLayout {
  return { boards: layout.boards.map((b) => ({ ...b, cards: b.cards.filter((c) => known.has(c.id)).map((c) => ({ ...c })) })) }
}

const replaceBoard = (layout: DashboardLayout, id: string, edit: (b: DashboardBoard) => DashboardBoard): DashboardLayout => ({
  boards: layout.boards.map((b) => (b.id === id ? edit(b) : b)),
})

const replaceCards = (layout: DashboardLayout, id: string, edit: (cards: DashboardPlacement[]) => DashboardPlacement[]): DashboardLayout =>
  replaceBoard(layout, id, (b) => ({ ...b, cards: edit(clone(b.cards)) }))

export function resizeCard(layout: DashboardLayout, boardId: string, cardId: string, size: { cols?: DashboardCols; rows?: DashboardRows }): DashboardLayout {
  return replaceCards(layout, boardId, (cards) => cards.map((c) => (c.id === cardId ? { ...c, ...size } : c)))
}

export function removeCard(layout: DashboardLayout, boardId: string, cardId: string): DashboardLayout {
  return replaceCards(layout, boardId, (cards) => cards.filter((c) => c.id !== cardId))
}

/** Moves the card at `from` to `to` (an index in the same list). Out-of-range or equal indexes change nothing. */
export function moveCard(layout: DashboardLayout, boardId: string, from: number, to: number): DashboardLayout {
  return replaceCards(layout, boardId, (cards) => {
    if (from === to || from < 0 || to < 0 || from >= cards.length || to >= cards.length) return cards
    const [moved] = cards.splice(from, 1)
    cards.splice(to, 0, moved!)
    return cards
  })
}

/** Adds a card at the end, at its own default size. A card already on the board, or a board at its limit, changes nothing. */
export function addCard(layout: DashboardLayout, boardId: string, card: DashboardPlacement): DashboardLayout {
  return replaceCards(layout, boardId, (cards) => (cards.length >= MAX_CARDS_PER_BOARD || cards.some((c) => c.id === card.id) ? cards : [...cards, { ...card }]))
}

/** Copies a card from a stack's board to Home at a small size, so a stack view feeds Home without being built twice. */
export function pinToHome(layout: DashboardLayout, cardId: string): DashboardLayout {
  return addCard(layout, HOME_BOARD_ID, { id: cardId, cols: PINNED_COLS, rows: 1 })
}

export const isOnBoard = (layout: DashboardLayout, boardId: string, cardId: string): boolean =>
  layout.boards.find((b) => b.id === boardId)?.cards.some((c) => c.id === cardId) ?? false

/** Re-flows one board: the shipped arrangement, or every card at one width. Order is kept except for `default`. */
export function applyPreset(layout: DashboardLayout, boardId: string, preset: DashboardPreset): DashboardLayout {
  if (preset === 'default') {
    const t = templateOf(boardId)
    return t ? replaceBoard(layout, boardId, (b) => ({ ...b, cards: clone(t.cards) })) : layout
  }
  const cols = PRESET_COLS[preset]
  return replaceCards(layout, boardId, (cards) => cards.map((c) => ({ ...c, cols, rows: 1 })))
}

const slug = (name: string): string => name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, MAX_BOARD_NAME) || 'board'

function freshId(layout: DashboardLayout, base: string): string {
  const taken = new Set(layout.boards.map((b) => b.id))
  if (!taken.has(base)) return base
  for (let n = 2; ; n++) {
    const candidate = `${base.slice(0, MAX_BOARD_NAME - String(n).length - 1)}-${n}`
    if (!taken.has(candidate)) return candidate
  }
}

/** A blank board, or one of the shipped boards. Returns the layout and the id of the board added, or the layout unchanged at the limit. */
export function addBoard(layout: DashboardLayout, template?: DashboardBoardTemplate): { layout: DashboardLayout; id: string | null } {
  if (layout.boards.length >= MAX_BOARDS) return { layout, id: null }
  const board = template ? boardFromTemplate(template) : { id: freshId(layout, slug(NEW_BOARD_NAME)), name: NEW_BOARD_NAME, cards: [] }
  if (template) board.id = freshId(layout, template.id)
  return { layout: { boards: [...layout.boards, board] }, id: board.id }
}

/** Home cannot be removed: it is where the dashboard opens. */
export function removeBoard(layout: DashboardLayout, boardId: string): DashboardLayout {
  return boardId === HOME_BOARD_ID ? layout : { boards: layout.boards.filter((b) => b.id !== boardId) }
}

/** An empty or over-long name is refused (the layout is returned as it was). */
export function renameBoard(layout: DashboardLayout, boardId: string, name: string): DashboardLayout {
  const trimmed = name.trim()
  if (!trimmed || trimmed.length > MAX_BOARD_NAME) return layout
  return replaceBoard(layout, boardId, (b) => ({ ...b, name: trimmed }))
}

const STACK_SOURCES: ReadonlySet<string> = new Set(STACK_IDS)

/**
 * Whether something that comes from `source` is worth offering. Host, assistant, automations and reminders are always there; a stack's
 * cards and board are offered only while the stack has a module on. `on` null means the list of stacks is not known: offer everything.
 */
export const sourceIsOn = (source: DashboardSource | null, on: ReadonlySet<string> | null): boolean =>
  on === null || source === null || !STACK_SOURCES.has(source) || on.has(source)

/** The cards the catalogue offers: those whose stack is on. */
export const cardsToOffer = (defs: readonly DashboardCardDef[], on: ReadonlySet<string> | null): DashboardCardDef[] => defs.filter((d) => sourceIsOn(d.source, on))

/** The shipped boards that are not on the dashboard now and whose stack is on, for the "Add board" menu. */
export function boardsToOffer(layout: DashboardLayout, on: ReadonlySet<string> | null = null): DashboardBoardTemplate[] {
  const present = new Set(layout.boards.map((b) => b.id))
  return BOARD_TEMPLATES.filter((t) => t.source !== null && !present.has(t.id) && sourceIsOn(t.source, on))
}
