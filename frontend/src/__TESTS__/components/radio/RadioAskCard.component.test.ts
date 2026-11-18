import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ ask: vi.fn(), markMine: vi.fn(), setClass: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioAskCard from '../../../components/radio/RadioAskCard.vue'

const mounted: Array<{ unmount: () => void }> = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })
beforeEach(() => {
  vi.clearAllMocks()
  service.ask.mockResolvedValue({ learned: { owner_labels: 2, agree: 1, checked: 2 }, devices: [
    { id: 'aa000001', mine: false, class: 'personal', heard: true, dbm: -48, reason: 'likely_yours', ident: { kind: 'phone', kind_confidence: 45, kind_source: 'vendor_element', brand: 'Apple', brand_hint: true, confidence: 60, on_your_wifi: true } },
  ] })
  service.markMine.mockResolvedValue({ label: 'x' })
  service.setClass.mockResolvedValue({ class: 'neighbour' })
})

async function show() {
  const w = mount(RadioAskCard, { global: { stubs: { RouterLink: { props: ['to'], template: '<a><slot /></a>' } } } })
  mounted.push(w)
  await flushPromises()
  return w
}

describe('RadioAskCard', () => {
  it('asks about a device that is likely yours and says what it learned', async () => {
    const w = await show()
    expect(w.findAll('[data-test="radio-ask-row"]')).toHaveLength(1)
    expect(w.text()).toContain('Learned from 2 of your answers; it gives back 1 of 2')
    expect(w.text()).toContain('may be yours')
  })

  it('sends "mine" under its make and kind and "not mine" for the server to place', async () => {
    const w = await show()
    await w.get('[data-test="radio-ask-mine"]').trigger('click')
    await flushPromises()
    expect(service.markMine).toHaveBeenCalledWith('aa000001', 'Apple phone or tablet')
    await w.get('[data-test="radio-ask-not-mine"]').trigger('click')
    await flushPromises()
    expect(service.setClass).toHaveBeenCalledWith('aa000001', 'not_mine')
  })

  it('hides itself when there is nothing to ask', async () => {
    service.ask.mockResolvedValue({ learned: {}, devices: [] })
    const w = await show()
    expect(w.find('[data-test="radio-ask"]').exists()).toBe(false)
  })
})
