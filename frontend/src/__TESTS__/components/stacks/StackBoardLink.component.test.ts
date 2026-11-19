import { describe, expect, it } from 'vitest'
import { RouterLinkStub, mount } from '@vue/test-utils'
import StackBoardLink from '../../../components/stacks/StackBoardLink.vue'
import { CARD_DEFS_BY_ID } from '../../../components/dashboard/cardRegistry'
import { BOARD_HINT } from '../../../constants/dashboard'
import type { BoardHintKey } from '../../../types/dashboard'

describe('StackBoardLink', () => {
  it.each(Object.keys(BOARD_HINT) as BoardHintKey[])('links the %s hint to its board, and to its card when it names one', (key) => {
    const w = mount(StackBoardLink, { props: { stack: key }, global: { stubs: { RouterLink: RouterLinkStub } } })
    const hint = BOARD_HINT[key]
    expect(w.getComponent(RouterLinkStub).props('to')).toEqual({ name: 'dashboard', query: { board: hint.board, ...(hint.card ? { card: hint.card } : {}) } })
  })

  it('names only cards the catalogue has, so "Add it" can always add what a page links to', () => {
    for (const hint of Object.values(BOARD_HINT)) if (hint.card) expect(CARD_DEFS_BY_ID.has(hint.card)).toBe(true)
  })

  it('sends the Network page to the timeline card, the card a saved board is most likely to lack', () => {
    expect(BOARD_HINT.network.card).toBe('network-timeline')
  })
})
