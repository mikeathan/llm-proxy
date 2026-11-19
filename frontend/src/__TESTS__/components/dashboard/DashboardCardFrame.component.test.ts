import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import DashboardCardFrame from '../../../components/dashboard/DashboardCardFrame.vue'
import Panel from '../../../components/common/layout/Panel.vue'
import { useCardSize } from '../../../composables/dashboard/useCardSize'
import { fakeMatchMedia } from '../../helpers/fakeMatchMedia'
import type { DashboardCardDef } from '../../../types/dashboard'

const Probe = defineComponent({
  setup() {
    const size = useCardSize()
    return () => h('div', { 'data-test': 'probe' }, [size.value, h(Panel, { title: 'Inner' }, () => 'inside')])
  },
})
const def: DashboardCardDef = { id: 'probe', source: 'network', title: 'Probe card', component: Probe, cols: 4, rows: 1, control: () => ({ name: 'network' }) }

let resize: ((width: number) => void) | undefined
class FakeObserver {
  constructor(cb: ResizeObserverCallback) { resize = (width) => cb([{ contentRect: { width } } as ResizeObserverEntry], this as unknown as ResizeObserver) }
  observe() {}
  disconnect() {}
}

const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }, { path: '/network', name: 'network', component: { template: '<div />' } }] })

function show(props: Partial<{ arrange: boolean; pinned: boolean; onHome: boolean; index: number; count: number }> = {}) {
  return mount(DashboardCardFrame, { props: { def, placement: { id: 'probe', cols: 4, rows: 1 }, arrange: false, index: 1, count: 3, pinned: false, onHome: false, ...props }, global: { plugins: [router] } })
}

let screen: ReturnType<typeof fakeMatchMedia>
beforeEach(() => {
  screen = fakeMatchMedia(1440)
  vi.stubGlobal('ResizeObserver', FakeObserver)
  vi.stubGlobal('matchMedia', screen.matchMedia)
})
afterEach(() => { vi.unstubAllGlobals(); resize = undefined })

describe('DashboardCardFrame', () => {
  it('draws the title, the source chip and the arrow to the control page', () => {
    const w = show()
    expect(w.text()).toContain('Probe card')
    expect(w.get('[data-test="card-source"]').text()).toBe('NETWORK')
    expect(w.get('[data-test="card-open"]').attributes('aria-label')).toBe('Open Probe card: control and settings')
  })

  it('is the panel: a Panel inside the card draws only its content', () => {
    const w = show()
    expect(w.findAll('section')).toHaveLength(1)
    expect(w.text()).toContain('inside')
    expect(w.text()).not.toContain('Inner')
  })

  it('gives the card the size its own width allows, and only that', async () => {
    const w = show()
    expect(w.get('[data-test="probe"]').text()).toContain('lg') // before it is measured nothing is hidden
    resize!(200)
    await nextTick()
    expect(w.get('[data-test="probe"]').text()).toContain('xs')
    resize!(400)
    await nextTick()
    expect(w.get('[data-test="probe"]').text()).toContain('sm')
    resize!(900)
    await nextTick()
    expect(w.get('[data-test="probe"]').text()).toContain('lg')
  })

  it('shows a card on a phone its full detail whatever its width: it is a full-width block of its own', async () => {
    screen.resize(390)
    const w = show()
    resize!(326)
    await nextTick()
    expect(w.get('[data-test="probe"]').text()).toContain('lg')
    screen.resize(1440)
    await nextTick()
    expect(w.get('[data-test="probe"]').text()).toContain('sm') // back on a wide screen the measured width decides
  })

  it('shows the arrange controls only while arranging', () => {
    expect(show().find('[data-test="card-controls"]').exists()).toBe(false)
    expect(show({ arrange: true }).find('[data-test="card-controls"]').exists()).toBe(true)
  })

  it('emits width, height, order, pin and remove from the arrange controls', async () => {
    const w = show({ arrange: true })
    const choose = (group: string, label: string) => w.get(`[aria-label="${group} of Probe card"]`).findAll('button').find((b) => b.text() === label)!.trigger('click')
    await choose('Width', 'L')
    await choose('Height', '2×')
    const moves = w.findAll('button[aria-label^="Move"]')
    await moves[0]!.trigger('click')
    await moves[1]!.trigger('click')
    await w.get('[data-test="card-pin"]').trigger('click')
    await w.get('[data-test="card-remove"]').trigger('click')
    expect(w.emitted('resize')).toEqual([[{ cols: 8 }], [{ rows: 2 }]])
    expect(w.emitted('move')).toEqual([[-1], [1]])
    expect(w.emitted('pin')).toHaveLength(1)
    expect(w.emitted('remove')).toHaveLength(1)
  })

  it('disables moving past either end, and pinning a card Home already has; Home has no pin', () => {
    expect(show({ arrange: true, index: 0 }).get('button[aria-label^="Move Probe card earlier"]').attributes('disabled')).toBeDefined()
    expect(show({ arrange: true, index: 2 }).get('button[aria-label^="Move Probe card later"]').attributes('disabled')).toBeDefined()
    const pinned = show({ arrange: true, pinned: true }).get('[data-test="card-pin"]')
    expect(pinned.text()).toBe('On Home')
    expect(pinned.attributes('disabled')).toBeDefined()
    expect(show({ arrange: true, onHome: true }).find('[data-test="card-pin"]').exists()).toBe(false)
  })
})
