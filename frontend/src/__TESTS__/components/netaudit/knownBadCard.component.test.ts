import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { KnownBadState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ knownBad: vi.fn(), saveKnownBadAddress: vi.fn(), removeKnownBadAddress: vi.fn(), downloadKnownBad: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import KnownBadCard from '../../../components/netaudit/KnownBadCard.vue'

const state = (over: Partial<KnownBadState> = {}): KnownBadState => ({ count: 0, has_address: false, ready: false, reason: 'no list address is saved yet', ...over })
const ADDRESS = 'https://urlhaus-api.abuse.ch/v2/files/exports/0123456789abcdef/hostfile.txt'

const mountCard = async () => {
  const w = mount(KnownBadCard, { global: { stubs: { RouterLink: { props: ['to'], template: '<a data-test="modules-link"><slot /></a>' } } } })
  await flushPromises()
  return w
}

describe('KnownBadCard', () => {
  beforeEach(() => { for (const f of Object.values(service)) f.mockReset(); service.knownBad.mockResolvedValue(state()) })

  it('explains itself, links to abuse.ch and keeps the how-to folded until asked', async () => {
    const w = await mountCard()
    expect(w.text()).toContain('Compares the names your devices ask for')
    expect(w.get('[data-test="known-bad-link"]').attributes('href')).toBe('https://urlhaus.abuse.ch/api/')
    expect(w.get('[data-test="known-bad-link"]').attributes('rel')).toContain('noopener')
    expect(w.find('[data-test="known-bad-howto"]').exists()).toBe(false)
    await w.get('[data-test="known-bad-howto-toggle"]').trigger('click')
    expect(w.get('[data-test="known-bad-howto"]').text()).toContain('Auth-Key')
  })

  it('keeps the address in a hidden field, saves it, clears the field and never shows it again', async () => {
    service.saveKnownBadAddress.mockResolvedValue(state({ has_address: true, reason: 'the list is not downloaded yet' }))
    const w = await mountCard()
    const field = w.get('[data-test="known-bad-address"]')
    expect(field.attributes('type')).toBe('password')
    await field.setValue(`  ${ADDRESS} `)
    await w.get('[data-test="known-bad-save"]').trigger('submit')
    await flushPromises()
    expect(service.saveKnownBadAddress).toHaveBeenCalledWith(ADDRESS)
    expect((field.element as HTMLInputElement).value).toBe('')
    expect(w.get('[data-test="known-bad-address-state"]').text()).toBe('An address is saved.')
    expect(w.html()).not.toContain('0123456789abcdef')
  })

  it('cannot save an empty address and cannot download without one', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="known-bad-save"]').attributes('disabled')).toBeDefined()
    expect(w.get('[data-test="known-bad-download"]').attributes('disabled')).toBeDefined()
  })

  it('downloads on request and says how many names are held', async () => {
    service.knownBad.mockResolvedValue(state({ has_address: true, reason: 'the list is not downloaded yet' }))
    service.downloadKnownBad.mockResolvedValue(state({ has_address: true, ready: true, reason: undefined, count: 453, loaded_at: new Date().toISOString() }))
    const w = await mountCard()
    expect(service.downloadKnownBad).not.toHaveBeenCalled()
    await w.get('[data-test="known-bad-download"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="known-bad-line"]').text()).toContain('453 malware host names held')
    expect(w.get('[data-test="known-bad-download"]').text()).toBe('Refresh list')
    expect(w.find('[data-test="known-bad-reason"]').exists()).toBe(false)
  })

  it('says why a download failed and keeps what it had', async () => {
    service.knownBad.mockResolvedValue(state({ has_address: true, count: 10, ready: true, reason: undefined }))
    service.downloadKnownBad.mockRejectedValue(new Error('abuse.ch answered 403: check that the saved address is the one on your URLhaus page'))
    const w = await mountCard()
    await w.get('[data-test="known-bad-download"]').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('403')
    expect(w.get('[data-test="known-bad-line"]').text()).toContain('10 malware host names held')
  })

  it('removes the saved address and points at the Modules tab for the switch', async () => {
    service.knownBad.mockResolvedValue(state({ has_address: true }))
    service.removeKnownBadAddress.mockResolvedValue(state())
    const w = await mountCard()
    expect(w.find('[data-test="modules-link"]').exists()).toBe(true)
    await w.get('[data-test="known-bad-remove"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="known-bad-address-state"]').text()).toBe('No address saved yet.')
  })
})
