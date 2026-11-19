import { describe, expect, it } from 'vitest'
import { addBoard, addCard, applyPreset, boardsToOffer, isOnBoard, moveCard, pinToHome, reconcile, removeBoard, removeCard, renameBoard, resizeCard, shippedLayout, sizeForWidth } from '../../domain/dashboard'
import { BOARD_TEMPLATES, HOME_BOARD_ID, MAX_BOARDS, MAX_CARDS_PER_BOARD } from '../../constants/dashboard'
import type { DashboardLayout } from '../../types/dashboard'

const everyCard = new Set(BOARD_TEMPLATES.flatMap((t) => t.cards.map((c) => c.id)))
const small: DashboardLayout = {
  boards: [
    { id: 'home', name: 'Home', cards: [{ id: 'a', cols: 4, rows: 1 }, { id: 'b', cols: 8, rows: 2 }, { id: 'c', cols: 3, rows: 1 }] },
    { id: 'network', name: 'Network', cards: [{ id: 'a', cols: 12, rows: 1 }] },
  ],
}
const ids = (l: DashboardLayout, board: string) => l.boards.find((b) => b.id === board)?.cards.map((c) => c.id)

describe('shippedLayout and reconcile', () => {
  it('starts with Home and every stack board, not the boards that carry no stack', () => {
    const ids = shippedLayout().boards.map((b) => b.id)
    expect(ids[0]).toBe(HOME_BOARD_ID)
    expect(ids).toEqual(BOARD_TEMPLATES.map((t) => t.id))
  })

  it('shows the shipped boards while nothing is stored', () => {
    expect(reconcile({ boards: [] }, everyCard)).toEqual(shippedLayout())
  })

  it('drops a card the screen has no definition for and keeps the rest in order', () => {
    const out = reconcile(small, new Set(['a', 'c']))
    expect(ids(out, 'home')).toEqual(['a', 'c'])
  })

  it('puts Home back when a stored layout has lost it', () => {
    const out = reconcile({ boards: [small.boards[1]!] }, everyCard)
    expect(out.boards[0]!.id).toBe(HOME_BOARD_ID)
    expect(out.boards).toHaveLength(2)
  })

  it('does not share card objects with the shipped templates', () => {
    const a = shippedLayout()
    a.boards[0]!.cards[0]!.cols = 3
    expect(shippedLayout().boards[0]!.cards[0]!.cols).not.toBe(3)
  })
})

describe('editing a board', () => {
  it('resizes one card on one board only', () => {
    const out = resizeCard(small, 'home', 'a', { cols: 12, rows: 2 })
    expect(out.boards[0]!.cards[0]).toEqual({ id: 'a', cols: 12, rows: 2 })
    expect(out.boards[1]!.cards[0]!.cols).toBe(12)
    expect(small.boards[0]!.cards[0]!.cols).toBe(4)
  })

  it('moves a card to a place in the same board', () => {
    expect(ids(moveCard(small, 'home', 0, 2), 'home')).toEqual(['b', 'c', 'a'])
    expect(ids(moveCard(small, 'home', 2, 0), 'home')).toEqual(['c', 'a', 'b'])
  })

  it.each([[0, 0], [-1, 1], [1, 9], [9, 0]])('leaves the order alone for a move from %i to %i', (from, to) => {
    expect(ids(moveCard(small, 'home', from, to), 'home')).toEqual(['a', 'b', 'c'])
  })

  it('removes a card', () => {
    expect(ids(removeCard(small, 'home', 'b'), 'home')).toEqual(['a', 'c'])
  })

  it('adds a card at the end once, and never past the limit', () => {
    const added = addCard(small, 'network', { id: 'z', cols: 4, rows: 1 })
    expect(ids(added, 'network')).toEqual(['a', 'z'])
    expect(ids(addCard(added, 'network', { id: 'z', cols: 4, rows: 1 }), 'network')).toEqual(['a', 'z'])
    const full: DashboardLayout = { boards: [{ id: 'home', name: 'Home', cards: Array.from({ length: MAX_CARDS_PER_BOARD }, (_, i) => ({ id: `c${i}`, cols: 4, rows: 1 })) }] }
    expect(addCard(full, 'home', { id: 'extra', cols: 4, rows: 1 }).boards[0]!.cards).toHaveLength(MAX_CARDS_PER_BOARD)
  })
})

describe('pinning to Home', () => {
  it('copies a card to Home at a small size', () => {
    const out = pinToHome({ boards: [{ id: 'home', name: 'Home', cards: [] }, small.boards[1]!] }, 'a')
    expect(out.boards[0]!.cards).toEqual([{ id: 'a', cols: 4, rows: 1 }])
    expect(isOnBoard(out, 'home', 'a')).toBe(true)
    expect(isOnBoard(out, 'network', 'a')).toBe(true)
  })

  it('does not pin a card Home already has', () => {
    expect(pinToHome(small, 'a').boards[0]!.cards).toHaveLength(3)
  })
})

describe('layout presets', () => {
  it.each([['compact', 3], ['two-columns', 6], ['one-column', 12]] as const)('%s puts every card at %i columns, one row tall, in the same order', (preset, cols) => {
    const out = applyPreset(small, 'home', preset)
    expect(out.boards[0]!.cards.map((c) => [c.id, c.cols, c.rows])).toEqual([['a', cols, 1], ['b', cols, 1], ['c', cols, 1]])
    expect(out.boards[1]).toEqual(small.boards[1])
  })

  it('goes back to the shipped arrangement of a shipped board', () => {
    const edited = removeCard(shippedLayout(), 'radio', 'radio-findings')
    expect(applyPreset(edited, 'radio', 'default')).toEqual(shippedLayout())
  })

  it('leaves a board that did not ship as it is', () => {
    const custom: DashboardLayout = { boards: [{ id: 'mine', name: 'Mine', cards: [{ id: 'a', cols: 3, rows: 1 }] }] }
    expect(applyPreset(custom, 'mine', 'default')).toEqual(custom)
  })
})

describe('boards', () => {
  it('adds a blank board with a fresh id each time', () => {
    const one = addBoard({ boards: [] })
    const two = addBoard(one.layout)
    expect(one.id).toBe('new-board')
    expect(two.id).toBe('new-board-2')
    expect(two.layout.boards.map((b) => b.name)).toEqual(['New board', 'New board'])
  })

  it('adds a shipped board with its cards', () => {
    const t = BOARD_TEMPLATES.find((b) => b.id === 'radio')!
    const out = addBoard({ boards: [] }, t)
    expect(out.layout.boards[0]!.cards.map((c) => c.id)).toEqual(t.cards.map((c) => c.id))
  })

  it('stops at the limit', () => {
    const full: DashboardLayout = { boards: Array.from({ length: MAX_BOARDS }, (_, i) => ({ id: `b${i}`, name: 'B', cards: [] })) }
    expect(addBoard(full)).toEqual({ layout: full, id: null })
  })

  it('never removes Home', () => {
    expect(removeBoard(small, HOME_BOARD_ID)).toBe(small)
    expect(removeBoard(small, 'network').boards.map((b) => b.id)).toEqual(['home'])
  })

  it('renames a board, trimmed, and refuses an empty or over-long name', () => {
    expect(renameBoard(small, 'network', '  Wi-Fi  ').boards[1]!.name).toBe('Wi-Fi')
    expect(renameBoard(small, 'network', '   ')).toBe(small)
    expect(renameBoard(small, 'network', 'x'.repeat(41))).toBe(small)
  })

  it('offers the stack boards that are not on the dashboard', () => {
    const without = removeBoard(shippedLayout(), 'radio')
    expect(boardsToOffer(without).map((t) => t.id)).toEqual(['radio'])
    expect(boardsToOffer(shippedLayout())).toEqual([])
  })
})

describe('sizeForWidth', () => {
  it.each([[100, 'xs'], [299, 'xs'], [300, 'sm'], [326, 'sm'], [559, 'sm'], [560, 'lg'], [1100, 'lg']] as const)('a card %ipx wide is %s', (px, size) => {
    expect(sizeForWidth(px)).toBe(size)
  })

  it.each([0, -5, Number.NaN, Number.POSITIVE_INFINITY])('counts %s, which is not a measured width, as large so nothing is hidden before layout', (px) => {
    expect(sizeForWidth(px)).toBe('lg')
  })
})
