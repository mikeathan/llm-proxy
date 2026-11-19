import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import DashboardGrid from '../../../components/dashboard/DashboardGrid.vue'
import { moveCard } from '../../../domain/dashboard'
import { fakeMatchMedia } from '../../helpers/fakeMatchMedia'
import type { DashboardCardDef, DashboardLayout } from '../../../types/dashboard'

const Body = defineComponent({ setup: () => () => h('div', 'body') })
const defs = new Map<string, DashboardCardDef>(
  ['a', 'b', 'c'].map((id) => [id, { id, source: 'network', title: 'Card ' + id, component: Body, cols: 4, rows: 1, control: () => ({ name: 'network' }) }]),
)
const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }, { path: '/network', name: 'network', component: { template: '<div />' } }] })

// The grid is driven the way the dashboard drives it: a move edits the layout, which flows back in as the board.
function harness() {
  const layout = ref<DashboardLayout>({ boards: [{ id: 'network', name: 'Network', cards: [{ id: 'a', cols: 4, rows: 1 }, { id: 'b', cols: 4, rows: 2 }, { id: 'c', cols: 4, rows: 1 }] }] })
  const w = mount(
    defineComponent({
      setup: () => () => h(DashboardGrid, { board: layout.value.boards[0]!, defs, arrange: true, pinned: () => false, onMove: (from: number, to: number) => { layout.value = moveCard(layout.value, 'network', from, to) } }),
    }),
    { attachTo: document.body, global: { plugins: [router] } },
  )
  const order = () => w.findAll('[data-test^="card-"]').filter((n) => n.attributes('data-test') !== 'card-controls' && n.attributes('data-test') !== 'card-source' && n.attributes('data-test') !== 'card-open' && n.attributes('data-test') !== 'card-remove' && n.attributes('data-test') !== 'card-pin').map((n) => n.attributes('data-test'))
  return { w, order }
}

beforeEach(() => {
  vi.stubGlobal('matchMedia', fakeMatchMedia(1440).matchMedia)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})
afterEach(() => vi.unstubAllGlobals())

describe('DashboardGrid arranging', () => {
  it('moves a card one place at a time and keeps focus on the button that was pressed', async () => {
    const { w, order } = harness()
    expect(order()).toEqual(['card-a', 'card-b', 'card-c'])
    for (const expected of [['card-b', 'card-a', 'card-c'], ['card-b', 'card-c', 'card-a']]) {
      await w.get('[data-test="card-a"] [data-move="later"]').trigger('click')
      await nextTick()
      expect(order()).toEqual(expected)
    }
    // Reached the end: "later" is disabled and focus moved to "earlier" so the keyboard is not stranded.
    expect((w.get('[data-test="card-a"] [data-move="later"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(document.activeElement).toBe(w.get('[data-test="card-a"] [data-move="earlier"]').element)
    w.unmount()
  })

  it('does not make the cell draggable as a whole, only its grip; and a tall card is a class (a minimum height), not a row span', () => {
    const { w } = harness()
    const cells = w.findAll('.dash-cell')
    expect(cells.some((c) => c.attributes('draggable') === 'true')).toBe(false)
    expect(w.findAll('[data-drag-grip]')).toHaveLength(3)
    expect(cells.map((c) => c.classes('dash-cell--tall'))).toEqual([false, true, false])
    expect(cells[1]!.attributes('style')).not.toContain('--rows')
    w.unmount()
  })

  it('drags a card from its grip onto another', async () => {
    const { w, order } = harness()
    await w.get('[data-test="card-a"] [data-drag-grip]').trigger('dragstart')
    await w.findAll('.dash-cell')[2]!.trigger('drop')
    expect(order()).toEqual(['card-b', 'card-c', 'card-a'])
    w.unmount()
  })
})
