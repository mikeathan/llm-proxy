import { describe, expect, it } from 'vitest'
import { RouterLinkStub, mount } from '@vue/test-utils'
import StackBoardLink from '../../../components/stacks/StackBoardLink.vue'
import { BOARD_HINT } from '../../../constants/dashboard'
import type { BoardHintKey } from '../../../types/dashboard'

describe('StackBoardLink', () => {
  it.each(Object.keys(BOARD_HINT) as BoardHintKey[])('links the %s hint to its board', (key) => {
    const w = mount(StackBoardLink, { props: { stack: key }, global: { stubs: { RouterLink: RouterLinkStub } } })
    const hint = BOARD_HINT[key]
    expect(w.getComponent(RouterLinkStub).props('to')).toEqual({ name: 'dashboard', query: { board: hint.board } })
  })

})
