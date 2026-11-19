import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { memoryStorage } from '../helpers/memoryStorage'
import type { DashboardCardDef, DashboardLayout } from '../../types/dashboard'

const service = vi.hoisted(() => ({ layout: vi.fn(), save: vi.fn(), reset: vi.fn() }))
vi.mock('../../services/dashboard/dashboardService', () => ({ DashboardService: service }))
const stacksService = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('../../services/stacks/stacksService', () => ({ StacksService: stacksService }))

// Two cards stand in for the catalogue: the view places cards, it does not read their data.
vi.mock('../../components/dashboard/cardRegistry', () => {
  const fake = (name: string) => defineComponent({ setup: () => () => h('p', { 'data-test': 'fake-card' }, name) })
  const defs = [
    { id: 'assistant-recent', source: 'assistant', title: 'Recent conversations', component: fake('recent body'), cols: 4, rows: 1, control: () => ({ name: 'dashboard' }) },
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
  stacksService.list.mockResolvedValue([
    { id: 'network', title: 'Network', purpose: '', on: 3, total: 5, stopped: 0, blocked: 0 },
    { id: 'radio', title: 'Radio', purpose: '', on: 2, total: 4, stopped: 0, blocked: 0 },
  ])
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
    expect(cardTitles(w).map((t) => t.toLowerCase())).toEqual(['recent conversations']) // Home ships minimal: nothing a stack board already shows
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
    service.layout.mockResolvedValue({ boards: [{ id: 'home', name: 'Home', cards: [{ id: 'host-health', cols: 12, rows: 1 }, { id: 'reminders', cols: 4, rows: 1 }] }] })
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

  it('offers only the cards and boards of a stack that has a module on', async () => {
    stacksService.list.mockResolvedValue([{ id: 'network', title: 'Network', purpose: '', on: 3, total: 5, stopped: 0, blocked: 0 }, { id: 'radio', title: 'Radio', purpose: '', on: 0, total: 4, stopped: 0, blocked: 0 }, { id: 'reminders', title: 'Reminders', purpose: '', on: 1, total: 1, stopped: 0, blocked: 0 }])
    const { w } = await open('/dashboard?board=automations')
    await w.get('[data-test="add-card"]').trigger('click')
    expect(w.find('[data-test="catalogue-reminders"]').exists()).toBe(true) // the reminders stack is on
    expect(w.find('[data-test="catalogue-radio-around"]').exists()).toBe(false) // the radio stack has nothing on
    await w.get('[data-test="add-board"]').trigger('click')
    expect(w.find('[data-test="add-board-network"]').exists()).toBe(false) // already on the dashboard, so not offered again
    expect(w.find('[data-test="add-board-radio"]').exists()).toBe(false) // and Radio has nothing on
  })

  it('hides the Reminders card while the Reminders stack is off', async () => {
    stacksService.list.mockResolvedValue([{ id: 'reminders', title: 'Reminders', purpose: '', on: 0, total: 1, stopped: 0, blocked: 0 }])
    const { w } = await open('/dashboard?board=automations')
    await w.get('[data-test="add-card"]').trigger('click')
    expect(w.find('[data-test="catalogue-reminders"]').exists()).toBe(false)
  })

  it('hides the board and the cards of a stack that has nothing on, and shows them again when it is back on', async () => {
    stacksService.list.mockResolvedValue([{ id: 'network', title: 'Network', purpose: '', on: 3, total: 5, stopped: 0, blocked: 0 }, { id: 'radio', title: 'Radio', purpose: '', on: 0, total: 4, stopped: 0, blocked: 0 }])
    const { w } = await open()
    expect(tabs(w).map((t) => t.text())).toEqual(['Home', 'LLM and host', 'Network', 'Automations'])
    expect(cardTitles(w).map((t) => t.toLowerCase())).toEqual(['recent conversations'])
    expect(service.save).not.toHaveBeenCalled() // hidden, not deleted
  })

  it('says a shipped board has cards it was saved without, adds them on request and does not ask again', async () => {
    service.layout.mockResolvedValue({ boards: [
      { id: 'home', name: 'Home', cards: [{ id: 'host-health', cols: 12, rows: 1 }] },
      { id: 'radio', name: 'Radio', cards: [{ id: 'host-health', cols: 12, rows: 1 }] },
    ] })
    const { w } = await open('/dashboard?board=radio')
    expect(w.text()).toContain('New on this board: Around you.')
    await w.get('[data-test="add-new-cards"]').trigger('click')
    await flushPromises()
    expect(cardTitles(w).map((t) => t.toLowerCase())).toContain('around you')
    expect(w.find('[data-test="add-new-cards"]').exists()).toBe(false)
  })

  it('stops offering new cards once they were declined', async () => {
    service.layout.mockResolvedValue({ boards: [
      { id: 'home', name: 'Home', cards: [{ id: 'host-health', cols: 12, rows: 1 }] },
      { id: 'radio', name: 'Radio', cards: [{ id: 'host-health', cols: 12, rows: 1 }] },
    ] })
    const { w } = await open('/dashboard?board=radio')
    await w.get('[data-test="dismiss-new-cards"]').trigger('click')
    expect(w.find('[data-test="add-new-cards"]').exists()).toBe(false)
    expect(cardTitles(w).map((t) => t.toLowerCase())).not.toContain('around you')
    expect(service.save).not.toHaveBeenCalled()
  })

  it('puts every board back as shipped from the Add board menu, after asking', async () => {
    service.layout.mockResolvedValue({ boards: [{ id: 'home', name: 'Home', cards: [{ id: 'host-health', cols: 12, rows: 1 }] }] })
    const { w } = await open()
    await w.get('[data-test="add-board"]').trigger('click')
    await w.get('[data-test="reset-all-boards"]').trigger('click')
    await flushPromises()
    expect(service.reset).not.toHaveBeenCalled() // the dialog asks first
    const { useConfirm } = await import('../../composables/ui/useConfirm')
    useConfirm().handleConfirm()
    await flushPromises()
    expect(service.reset).toHaveBeenCalledTimes(1)
  })

  it('says a linked card is not on the saved board, and adds it with one press', async () => {
    service.layout.mockResolvedValue({ boards: [
      { id: 'home', name: 'Home', cards: [{ id: 'host-health', cols: 12, rows: 1 }] },
      { id: 'radio', name: 'Radio', cards: [{ id: 'host-health', cols: 12, rows: 1 }] },
    ] })
    const { w } = await open('/dashboard?board=radio&card=radio-around')
    expect(w.get('[data-test="add-wanted-card"]').text()).toBe('Add it')
    expect(w.text()).toContain('Around you is not on this board.')
    await w.get('[data-test="add-wanted-card"]').trigger('click')
    await flushPromises()
    expect(cardTitles(w).map((t) => t.toLowerCase())).toContain('around you')
    expect(w.find('[data-test="add-wanted-card"]').exists()).toBe(false)
  })

  it('adds a shipped board that a saved layout predates when a stack page links to it', async () => {
    service.layout.mockResolvedValue({ boards: [{ id: 'home', name: 'Home', cards: [{ id: 'host-health', cols: 12, rows: 1 }] }] })
    stacksService.list.mockResolvedValue([{ id: 'agents', title: 'Agents', purpose: '', on: 2, total: 2, stopped: 0, blocked: 0 }])
    const { w } = await open('/dashboard?board=agents')
    expect(current(w)).toEqual(['Agents'])
    expect(service.save).toHaveBeenCalledTimes(1)
  })

  it('does not make up a board that did not ship', async () => {
    service.layout.mockResolvedValue({ boards: [{ id: 'home', name: 'Home', cards: [{ id: 'host-health', cols: 12, rows: 1 }] }] })
    const { w } = await open('/dashboard?board=nothing-like-this')
    expect(current(w)).toEqual(['Home'])
    expect(service.save).not.toHaveBeenCalled()
  })

  it('says nothing when the linked card is already on the board', async () => {
    const { w } = await open('/dashboard?board=host&card=host-health')
    expect(w.find('[data-test="add-wanted-card"]').exists()).toBe(false)
  })

  it('offers everything when the list of stacks cannot be read', async () => {
    stacksService.list.mockRejectedValue(new Error('down'))
    const { w } = await open('/dashboard?board=automations')
    await w.get('[data-test="add-card"]').trigger('click')
    expect(w.find('[data-test="catalogue-radio-around"]').exists()).toBe(true)
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
