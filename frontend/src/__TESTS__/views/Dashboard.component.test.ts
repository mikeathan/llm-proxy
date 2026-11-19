import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { memoryStorage } from '../helpers/memoryStorage'
import type { DashboardCardDef, DashboardLayout } from '../../types/dashboard'

const service = vi.hoisted(() => ({ layout: vi.fn(), save: vi.fn(), reset: vi.fn() }))
vi.mock('../../services/dashboard/dashboardService', () => ({ DashboardService: service }))

// Two cards stand in for the catalogue: the view places cards, it does not read their data.
vi.mock('../../components/dashboard/cardRegistry', () => {
  const fake = (name: string) => defineComponent({ setup: () => () => h('p', { 'data-test': 'fake-card' }, name) })
  const defs = [
    { id: 'host-health', source: 'host', title: 'Host health', component: fake('host body'), cols: 12, rows: 1, control: () => ({ name: 'dashboard' }) },
    { id: 'radio-around', source: 'radio', title: 'Around you', component: fake('radar body'), cols: 4, rows: 1, control: () => ({ name: 'dashboard' }) },
    { id: 'reminders', source: 'reminders', title: 'Reminders', component: fake('reminders body'), cols: 4, rows: 1, control: () => ({ name: 'dashboard' }) },
  ] as unknown as DashboardCardDef[]
  return { CARD_DEFS: defs, CARD_DEFS_BY_ID: new Map(defs.map((d) => [d.id, d])), KNOWN_CARD_IDS: new Set(defs.map((d) => d.id)) }
})

import DashboardView from '../../views/DashboardView.vue'
import { ROUTE_NAMES } from '../../types/routes'

const empty = { template: '<div />' }
async function show(path = '/dashboard') {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/dashboard', name: ROUTE_NAMES.dashboard, component: DashboardView }, { path: '/:rest(.*)*', component: empty }] })
  await router.push(path)
  const w = mount(DashboardView, { global: { plugins: [router] }, attachTo: document.body })
  await flushPromises()
  return { w, router }
}
const tabs = (w: Awaited<ReturnType<typeof show>>['w']) => w.findAll('[data-test="board-tab"]')
const current = (w: Awaited<ReturnType<typeof show>>['w']) => tabs(w).filter((t) => t.attributes('aria-current') === 'page').map((t) => t.text())
const cardTitles = (w: Awaited<ReturnType<typeof show>>['w']) => w.findAll('[data-test="dashboard-grid"] h2').map((h) => h.text().replace(/^[\s\d-]+/, ''))

let mounted: Array<{ unmount: () => void }> = []
beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('localStorage', memoryStorage())
  service.layout.mockResolvedValue({ boards: [] })
  service.save.mockImplementation(async (l: DashboardLayout) => l)
  service.reset.mockResolvedValue({ boards: [] })
})
afterEach(() => {
  mounted.forEach((w) => w.unmount())
  mounted = []
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})
async function open(path?: string) {
  const r = await show(path)
  mounted.push(r.w)
  return r
}

describe('DashboardView', () => {
  it('opens on Home with the shipped boards, drawing only the cards it knows', async () => {
    const { w } = await open()
    expect(tabs(w).map((t) => t.text())).toEqual(['Home', 'LLM and host', 'Network', 'Radio', 'Automations'])
    expect(current(w)).toEqual(['Home'])
    expect(cardTitles(w).map((t) => t.toLowerCase())).toEqual(['host health', 'around you', 'reminders'])
    expect(service.save).not.toHaveBeenCalled() // nothing is stored until the owner changes something
  })

  it('switches board from the tabs, and from ?board= (the stack pages link to theirs)', async () => {
    const { w, router } = await open()
    await tabs(w)[3]!.trigger('click')
    expect(current(w)).toEqual(['Radio'])
    await router.push('/dashboard?board=host')
    await flushPromises()
    expect(current(w)).toEqual(['LLM and host'])
  })

  it('opens the board named in the address on arrival', async () => {
    const { w } = await open('/dashboard?board=radio')
    expect(current(w)).toEqual(['Radio'])
  })

  it('arranges: resizing a card saves the layout and the board reads Custom', async () => {
    const { w } = await open()
    await w.get('[data-test="arrange"]').trigger('click')
    const group = w.get('[aria-label="Width of Host health"]')
    await group.findAll('button').find((b) => b.text() === 'S')!.trigger('click')
    await flushPromises()
    expect(service.save).toHaveBeenCalledTimes(1)
    const saved = service.save.mock.calls[0]![0] as DashboardLayout
    expect(saved.boards[0]!.cards[0]).toEqual({ id: 'host-health', cols: 3, rows: 1 })
    expect((w.get('[data-test="layout-select"]').element as HTMLSelectElement).value).toBe('custom')
  })

  it('re-flows the board from the layout menu and can put it back', async () => {
    const { w } = await open()
    await w.get('[data-test="layout-select"]').setValue('one-column')
    expect((saved(0) as DashboardLayout).boards[0]!.cards.every((c) => c.cols === 12)).toBe(true)
    await w.get('[data-test="reset-board"]').trigger('click')
    expect((w.get('[data-test="layout-select"]').element as HTMLSelectElement).value).toBe('default')
  })

  it('adds a blank board and opens it', async () => {
    const { w } = await open()
    await w.get('[data-test="add-board"]').trigger('click')
    await w.get('[data-test="add-blank-board"]').trigger('click')
    await flushPromises()
    expect(current(w)).toEqual(['New board'])
    expect(w.text()).toContain('This board is empty')
  })

  it('adds a card from the catalogue to the open board', async () => {
    const { w } = await open('/dashboard?board=automations')
    await w.get('[data-test="add-card"]').trigger('click')
    await w.get('[data-test="catalogue-radio-around"] button').trigger('click')
    await flushPromises()
    expect(cardTitles(w).map((t) => t.toLowerCase())).toContain('around you')
    await w.get('[data-test="add-card"]').trigger('click')
    expect(w.get('[data-test="catalogue-radio-around"]').text()).toContain('On this board') // it closes after an add; opened again it says so
  })

  it('says why it shows the standard boards when the layout cannot be read', async () => {
    service.layout.mockRejectedValue(new Error('the server is down'))
    const { w } = await open()
    expect(w.get('[data-test="dashboard-error"]').text()).toBe('the server is down')
    expect(tabs(w)).toHaveLength(5)
  })
})

function saved(call: number) {
  return service.save.mock.calls[call]![0]
}
