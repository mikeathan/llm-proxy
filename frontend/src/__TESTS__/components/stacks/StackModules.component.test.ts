import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { StackView, SwitchedStack } from '../../../types/stacks'

const service = vi.hoisted(() => ({ stack: vi.fn(), setModule: vi.fn() }))
vi.mock('../../../services/stacks/stacksService', () => ({ StacksService: service }))

import StackModules from '../../../components/stacks/StackModules.vue'

const network = (): StackView => ({
  id: 'network', title: 'Network', purpose: '', actions: [], summary: { on: 1, total: 3, stopped: 1, blocked: 1 },
  layers: [
    { id: 'tell', title: 'Tell', modules: [{ id: 'weekly_summary', title: 'Weekly summary', purpose: 'Counts of the week to your phone.', about: { watches: 'w.', never: 'n.', shows_up: 's.' }, finds: [], sends: 'message', step_up: false, enabled: false, blocked: 'needs Presence check', needs: ['presence'] }] },
    { id: 'discover', title: 'Discover', modules: [
      { id: 'presence', title: 'Presence check', purpose: 'Notice new and missing devices.', about: { watches: 'w.', never: 'n.', shows_up: 's.' }, finds: [], sends: 'lan', step_up: false, enabled: true, health: 'stopped', needs: [] },
      { id: 'nearby', title: 'Nearby', purpose: 'What devices announce.', about: { watches: 'w.', never: 'n.', shows_up: 's.' }, finds: [], sends: 'lan', step_up: true, enabled: false, needs: [] },
    ] },
  ],
})

let wrapper: VueWrapper | undefined
const mountIt = async () => {
  wrapper = mount(StackModules, { props: { stack: 'network' } })
  await flushPromises()
  return wrapper
}

describe('StackModules', () => {
  beforeEach(() => { service.stack.mockReset(); service.setModule.mockReset(); service.stack.mockResolvedValue(network()) })
  afterEach(() => { wrapper?.unmount(); wrapper = undefined })

  it('draws the layers top first, numbered from the bottom', async () => {
    const w = await mountIt()
    const headings = w.findAll('h3').map((h) => h.text().replace(/\s+/g, ' '))
    expect(headings).toEqual(['02 — Tell', '01 — Discover'])
  })

  it('gives each module its name, purpose and one status line', async () => {
    const w = await mountIt()
    const tile = w.get('[data-test="module-weekly_summary"]')
    expect(tile.text()).toContain('Weekly summary')
    expect(tile.text()).toContain('Counts of the week to your phone.')
    expect(tile.get('[data-test="module-status"]').text()).toBe('Off · sends a message · needs Presence check')
    expect(w.get('[data-test="module-presence"] [data-test="module-status"]').text()).toBe('Stopped · LAN')
  })

  it('dims a blocked module', async () => {
    const w = await mountIt()
    expect(w.get('[data-test="module-weekly_summary"]').classes()).toContain('opacity-70')
    expect(w.get('[data-test="module-presence"]').classes()).not.toContain('opacity-70')
  })

  it('sums the stack in one line', async () => {
    const w = await mountIt()
    expect(w.get('[data-test="stack-summary"]').text().replace(/\s+/g, ' ')).toBe('1 of 3 on · 1 stopped · 1 blocked')
  })

  it('switches a module through the server and shows the answer', async () => {
    const after: SwitchedStack = { ...network(), blocked: [] }
    after.layers[1]!.modules[1]!.enabled = true
    service.setModule.mockResolvedValue(after)
    const w = await mountIt()
    await w.get('[data-test="module-nearby"] input[role="switch"]').setValue(true)
    await flushPromises()
    expect(service.setModule).toHaveBeenCalledWith('network', 'nearby', true)
    expect((w.get('[data-test="module-nearby"] input[role="switch"]').element as HTMLInputElement).checked).toBe(true)
  })

  it('puts the switch back and says why when the server refuses it', async () => {
    service.setModule.mockRejectedValue(new Error('needs Presence check'))
    const w = await mountIt()
    const box = w.get('[data-test="module-weekly_summary"] input[role="switch"]')
    await box.setValue(true)
    await flushPromises()
    expect((box.element as HTMLInputElement).checked).toBe(false)
    expect(w.get('[data-test="module-weekly_summary"] [data-test="module-error"]').text()).toBe('needs Presence check')
  })

  it('names what a switch-off left waiting', async () => {
    service.setModule.mockResolvedValue({ ...network(), blocked: ['Weekly summary'] })
    const w = await mountIt()
    await w.get('[data-test="module-presence"] input[role="switch"]').setValue(false)
    await flushPromises()
    expect(w.get('[data-test="left-blocked"]').text()).toBe('Now waiting for it: Weekly summary.')
  })

  it('gives every switch the module\'s name as its accessible name', async () => {
    const w = await mountIt()
    const names = w.findAll('input[role="switch"]').map((i) => i.element.closest('label')?.textContent?.trim())
    expect(names).toEqual(['Weekly summary', 'Presence check', 'Nearby'])
  })

  it('offers a retry when the stack cannot be read', async () => {
    service.stack.mockRejectedValueOnce(new Error('down')).mockResolvedValue(network())
    const w = await mountIt()
    expect(w.text()).toContain('The modules could not be read')
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.find('[data-test="stack-modules"] h3').exists()).toBe(true)
  })
})
