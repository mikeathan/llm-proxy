import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { OutsideLookupState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ outsideLookup: vi.fn(), setOutsideLookup: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import OutsideLookupCard from '../../../components/netaudit/OutsideLookupCard.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const NEVER = '0001-01-01T00:00:00Z'
const state = (over: Partial<OutsideLookupState> = {}, last: Partial<OutsideLookupState['last']> = {}): OutsideLookupState => ({
  enabled: false,
  url: 'https://internetdb.shodan.io',
  default_url: 'https://internetdb.shodan.io',
  last: { at: NEVER, ports: 0, ...last },
  ...over,
})

async function mountCard() {
  const w = mount(OutsideLookupCard, {
    attachTo: document.body,
    global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="JSON.stringify(to)"><slot /></a>' } } },
  })
  await flushPromises()
  return w
}

describe('OutsideLookupCard', () => {
  beforeEach(() => {
    service.outsideLookup.mockReset().mockResolvedValue(state())
    service.setOutsideLookup.mockReset()
    useNetauditViewMode().set('simple')
  })

  it('is off until the person turns it on, and says nothing about a look that never happened', async () => {
    const w = await mountCard()
    expect((w.get('[role="switch"]').element as HTMLInputElement).checked).toBe(false)
    expect(w.find('[data-test="outside-lookup-status"]').exists()).toBe(false)
    w.unmount()
  })

  it('tells the person, before they switch it on, that the outside service learns the public address', async () => {
    const w = await mountCard()
    await w.get('[data-test="toggle-details"]').trigger('click')
    expect(w.get('[data-test="toggle-details-text"]').text()).toContain('learns your public internet address')
    w.unmount()
  })

  it('turns it on through the service and shows the state the server confirmed', async () => {
    service.setOutsideLookup.mockResolvedValue(state({ enabled: true }))
    const w = await mountCard()
    await w.get('[role="switch"]').setValue(true)
    await flushPromises()
    expect(service.setOutsideLookup).toHaveBeenCalledWith({ enabled: true })
    expect(w.get('[data-test="outside-lookup-status"]').text()).toContain('Not looked yet')
    w.unmount()
  })

  it('says what a look found, and where to read it', async () => {
    service.outsideLookup.mockResolvedValue(state({ enabled: true }, { at: new Date().toISOString(), outcome: 'looked', ports: 2 }))
    const w = await mountCard()
    const line = w.get('[data-test="outside-lookup-status"]').text()
    expect(line).toContain('2 open ports')
    expect(line).toContain('Findings')
    w.unmount()
  })

  it('says plainly when there was nothing to find, as the outside service sees it', async () => {
    service.outsideLookup.mockResolvedValue(state({ enabled: true }, { at: new Date().toISOString(), outcome: 'looked', ports: 0 }))
    const w = await mountCard()
    expect(w.get('[data-test="outside-lookup-status"]').text()).toContain('lists no open ports')
    w.unmount()
  })

  it('says why it could not look, that nothing was sent, and that this is not "safe"', async () => {
    service.outsideLookup.mockResolvedValue(state({ enabled: true }, { at: new Date().toISOString(), outcome: 'cannot_look', reason: 'private_address' }))
    const w = await mountCard()
    const line = w.get('[data-test="outside-lookup-status"]').text()
    expect(line).toContain('another router')
    expect(line).toContain('Nothing was sent')
    expect(line).toContain('does not mean nothing is open')
    w.unmount()
  })

  it('says when the outside service could not be reached', async () => {
    service.outsideLookup.mockResolvedValue(state({ enabled: true }, { at: new Date().toISOString(), outcome: 'failed', reason: 'lookup_failed' }))
    const w = await mountCard()
    expect(w.get('[data-test="outside-lookup-status"]').text()).toContain('could not be reached')
    w.unmount()
  })

  it('points to the network access setting when internet access is off, and only then', async () => {
    service.outsideLookup.mockResolvedValue(state({ enabled: true }, { at: new Date().toISOString(), outcome: 'failed', reason: 'internet_off', ports: 0 }))
    const w = await mountCard()
    expect(w.get('[data-test="outside-lookup-status"]').text()).toContain('internet access is off')
    expect(w.get('[data-test="outside-lookup-settings"]').attributes('href')).toContain('guardrails')
    w.unmount()
    service.outsideLookup.mockResolvedValue(state({ enabled: true }, { at: new Date().toISOString(), outcome: 'failed', reason: 'lookup_failed', ports: 0 }))
    const other = await mountCard()
    expect(other.find('[data-test="outside-lookup-settings"]').exists()).toBe(false)
    other.unmount()
  })

  it('shows a refusal (such as a missing passphrase) and keeps the switch where it was', async () => {
    service.setOutsideLookup.mockRejectedValue(new Error('step-up required'))
    const w = await mountCard()
    await w.get('[role="switch"]').setValue(true)
    await flushPromises()
    expect(w.text()).toContain('step-up required')
    w.unmount()
  })

  it('offers the service address only in the Advanced view, and saves it on its own', async () => {
    const w = await mountCard()
    expect(w.find('[data-test="outside-lookup-url"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    await flushPromises()
    service.setOutsideLookup.mockResolvedValue(state({ url: 'https://lookup.example.org' }))
    await w.get('[data-test="outside-lookup-url"]').setValue('https://lookup.example.org')
    await w.get('[data-test="outside-lookup-save"]').trigger('click')
    await flushPromises()
    expect(service.setOutsideLookup).toHaveBeenCalledWith({ url: 'https://lookup.example.org' })
    w.unmount()
  })

  it('shows why an address was refused', async () => {
    useNetauditViewMode().set('advanced')
    service.setOutsideLookup.mockRejectedValue(new Error('the service address must start with https://'))
    const w = await mountCard()
    await w.get('[data-test="outside-lookup-url"]').setValue('http://x')
    await w.get('[data-test="outside-lookup-save"]').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('must start with https://')
    w.unmount()
  })
})
