import { describe, expect, it } from 'vitest'
import { addBoard, addCard, applyPreset, boardsToOffer, cardsToOffer, sourceIsOn, isOnBoard, moveCard, pinToHome, reconcile, removeBoard, removeCard, renameBoard, resizeCard, shippedLayout, sizeForWidth, visibleLayout, newShippedCards, boardTabs } from '../../domain/dashboard'
import { BOARD_TEMPLATES, HOME_BOARD_ID, MAX_BOARDS, MAX_CARDS_PER_BOARD } from '../../constants/dashboard'
import type { DashboardBoard, DashboardCardDef, DashboardLayout } from '../../types/dashboard'

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

  describe('stacks that are off', () => {
    const def = (id: string, source: string) => ({ id, source }) as unknown as DashboardCardDef
    const defs = [def('h', 'host'), def('n', 'network'), def('r', 'radio'), def('a', 'automations')]

    it('offers every card while the list of stacks is not known', () => {
      expect(cardsToOffer(defs, null).map((d) => d.id)).toEqual(['h', 'n', 'r', 'a'])
    })

    it('offers a stack card only while the stack has a module on, and every other card always', () => {
      expect(cardsToOffer(defs, new Set(['network'])).map((d) => d.id)).toEqual(['h', 'n', 'a'])
      expect(cardsToOffer(defs, new Set()).map((d) => d.id)).toEqual(['h', 'a'])
      expect(sourceIsOn(null, new Set())).toBe(true)
    })

    it('offers a stack board only while the stack is on', () => {
      const without: DashboardLayout = { boards: shippedLayout().boards.filter((b) => b.id !== 'radio' && !b.id.startsWith('network')) }
      expect(boardsToOffer(without, new Set(['radio'])).map((t) => t.id)).toEqual(['radio'])
      expect(boardsToOffer(without, new Set()).map((t) => t.id)).toEqual([])
      expect(boardsToOffer(without, null).map((t) => t.id)).toEqual(['network', 'radio'])
    })
  })
})

describe('visibleLayout and newShippedCards', () => {
  const def = (id: string, source: DashboardCardDef['source']) => [id, { id, source } as DashboardCardDef] as const
  const defs = new Map([def('a', 'host'), def('n', 'network'), def('r', 'radio'), def('network-posture', 'network'), def('network-overview', 'network')])
  const layout: DashboardLayout = {
    boards: [
      { id: 'home', name: 'Home', cards: [{ id: 'a', cols: 4, rows: 1 }, { id: 'n', cols: 4, rows: 1 }, { id: 'r', cols: 4, rows: 1 }] },
      { id: 'network', name: 'Network', cards: [{ id: 'n', cols: 12, rows: 1 }] },
      { id: 'mine', name: 'Mine', cards: [{ id: 'r', cols: 6, rows: 1 }, { id: 'a', cols: 6, rows: 1 }] },
    ],
  }

  it('shows everything while the stacks are not known', () => {
    expect(visibleLayout(layout, defs, null)).toBe(layout)
  })

  it('hides the board and the cards of a stack that is off, and keeps the owner\'s other cards', () => {
    const shown = visibleLayout(layout, defs, new Set(['network']))
    expect(shown.boards.map((b) => b.id)).toEqual(['home', 'network', 'mine'])
    expect(ids(shown, 'home')).toEqual(['a', 'n'])
    expect(ids(shown, 'mine')).toEqual(['a'])
    expect(visibleLayout(layout, defs, new Set()).boards.map((b) => b.id)).toEqual(['home', 'mine']) // no Network board
    expect(layout.boards[0]!.cards).toHaveLength(3) // the saved layout is not changed
  })

  it('lists the cards a shipped board now has and the saved copy lacks, except those already offered', () => {
    const saved: DashboardBoard = { id: 'network', name: 'Network', cards: [{ id: 'network-posture', cols: 8, rows: 1 }] }
    expect(newShippedCards(saved, [], null, defs)).toEqual(['network-overview'])
    expect(newShippedCards(saved, ['network-overview'], null, defs)).toEqual([])
    expect(newShippedCards(saved, [], new Set(), defs)).toEqual([]) // the stack is off
    expect(newShippedCards({ id: 'mine', name: 'Mine', cards: [] } as DashboardBoard, [], null, defs)).toEqual([]) // a board that did not ship has no shipped cards
  })
})

describe('the Today card reaches a Home saved before it existed', () => {
  const defs = new Map([['assistant-recent', { id: 'assistant-recent', source: 'assistant' } as DashboardCardDef], ['newsroom-today', { id: 'newsroom-today', source: 'newsroom' } as DashboardCardDef]])
  const saved: DashboardBoard = { id: 'home', name: 'Home', cards: [{ id: 'assistant-recent', cols: 6, rows: 1 }] }

  it('is offered through the "New on this board" notice while the Newsroom is on, and not while it is off', () => {
    expect(newShippedCards(saved, [], new Set(['newsroom']), defs)).toEqual(['newsroom-today'])
    expect(newShippedCards(saved, [], new Set(['network']), defs)).toEqual([])
    expect(newShippedCards(saved, ['newsroom-today'], new Set(['newsroom']), defs)).toEqual([])
  })
})

describe("boardTabs: a stack's boards are one tab with sub-tabs", () => {
  const shipped = shippedLayout()
  const on = new Set(['network', 'radio'])

  it('shows Network once, with every Network board as a sub-tab, in the shipped order', () => {
    const tabs = boardTabs(shipped.boards, on)
    expect(tabs.map((t) => t.label)).toEqual(['Home', 'LLM and host', 'Network', 'Radio', 'Newsroom', 'Agents', 'Automations'])
    const network = tabs.find((t) => t.group === 'network')!
    expect(network.subTabs.map((s) => s.label)).toEqual(['Overview', 'Devices', 'Traffic', 'Findings', 'Activity'])
    expect(network.id).toBe('network')
    expect(network.subTabs.every((s) => s.present)).toBe(true)
  })

  it('lists a sub-tab the saved layout lacks, marked absent, so opening it can add it', () => {
    const saved = { boards: shipped.boards.filter((b) => b.id !== 'network-traffic') }
    const network = boardTabs(saved.boards, on).find((t) => t.group === 'network')!
    expect(network.subTabs.find((s) => s.id === 'network-traffic')).toMatchObject({ label: 'Traffic', present: false })
  })

  it('opens the first board of the group the layout has when the group tab is chosen', () => {
    const saved = { boards: shipped.boards.filter((b) => b.id !== 'network') }
    expect(boardTabs(saved.boards, on).find((t) => t.group === 'network')!.id).toBe('network-devices')
  })

  it('has no Network tab when no Network board is on the layout or the stack is off', () => {
    expect(boardTabs(shipped.boards.filter((b) => !b.id.startsWith('network')), on).some((t) => t.group === 'network')).toBe(false)
    expect(boardTabs(shipped.boards, new Set(['radio'])).some((t) => t.group === 'network')).toBe(false)
  })

  it("leaves a board of the owner's own as one tab with its own name", () => {
    const own = { id: 'mine', name: 'My board', cards: [] }
    expect((() => { const t = boardTabs([...shipped.boards, own], on); return t[t.length - 1] })()).toMatchObject({ id: 'mine', label: 'My board', subTabs: [] })
  })
})

describe('boards offered when a group is on the layout', () => {
  it('offers a group once, as its first board, and not at all when any of it is on the layout', () => {
    const without = { boards: shippedLayout().boards.filter((b) => !b.id.startsWith('network')) }
    expect(boardsToOffer(without).filter((t) => t.group === 'network').map((t) => t.id)).toEqual(['network'])
    expect(boardsToOffer({ boards: shippedLayout().boards.filter((b) => b.id !== 'network-traffic') }).some((t) => t.group === 'network')).toBe(false)
  })
})
