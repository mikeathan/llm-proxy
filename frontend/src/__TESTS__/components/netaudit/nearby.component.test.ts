import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { NearbyDevice, NearbyState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ nearby: vi.fn(), setNearby: vi.fn(), nearbyLook: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))
const clipboard = vi.hoisted(() => ({ copyText: vi.fn() }))
vi.mock('../../../utils/clipboard', () => clipboard)

import NearbySection from '../../../components/netaudit/NearbySection.vue'
import NearbyCard from '../../../components/netaudit/NearbyCard.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const LOOK = new Date().toISOString()
const HOSTILE = '<img src=x onerror="window.__pwned=1">‮gpj.exe'
const dev = (over: Partial<NearbyDevice> = {}): NearbyDevice => ({ ip: '192.168.50.20', first_heard: LOOK, last_heard: LOOK, known: true, new: false, ...over })
const state = (devices: NearbyDevice[], over: Partial<NearbyState> = {}): NearbyState => ({ enabled: true, running: false, last_look: LOOK, last_outcome: 'heard', devices, ...over })
const guide = { steps: ['Find the device.'], risk: 'It may stop.', undo: 'Put it back.' }
const rogue = dev({ ip: '192.168.50.7', gateway: true, names: ['Upstairs box'], problem: { check: 'rogue_gateway', title: 'A second router-like device', text: 'This device says it is an internet gateway, and it is not your router.', guide } })

const link = { RouterLink: { props: ['to'], template: '<a :href="JSON.stringify(to)"><slot /></a>' } }
async function mountSection() {
  const w = mount(NearbySection, { attachTo: document.body, global: { stubs: link } })
  await flushPromises()
  return w
}

describe('Nearby section', () => {
  beforeEach(() => {
    service.nearby.mockReset().mockResolvedValue(state([]))
    service.setNearby.mockReset()
    service.nearbyLook.mockReset()
    clipboard.copyText.mockReset().mockResolvedValue(true)
    useNetauditViewMode().set('simple')
  })

  it('off: says exactly what is sent and sends the person to the switch in Settings, not a second switch here', async () => {
    service.nearby.mockResolvedValue(state([], { enabled: false, last_look: undefined }))
    const w = await mountSection()
    expect(w.get('[data-test="nearby-off"]').text()).toContain('two small questions')
    expect(w.get('[data-test="nearby-turn-on"]').attributes('href')).toContain('settings')
    expect(w.find('[data-test="nearby-check"]').exists()).toBe(false)
    w.unmount()
  })

  it('never run: invites Check now', async () => {
    service.nearby.mockResolvedValue(state([], { last_look: undefined, last_outcome: undefined }))
    const w = await mountSection()
    expect(w.get('[data-test="nearby-summary"]').text()).toContain('has not looked yet')
    w.unmount()
  })

  it('nothing heard: says silence is not proof', async () => {
    service.nearby.mockResolvedValue(state([], { last_outcome: 'nothing_heard' }))
    const w = await mountSection()
    expect(w.get('[data-test="nearby-summary"]').text()).toContain('does not mean nothing is there')
    w.unmount()
  })

  it('failed: gives a plain reason', async () => {
    service.nearby.mockResolvedValue(state([], { last_outcome: 'lan_off' }))
    const w = await mountSection()
    expect(w.get('[data-test="nearby-summary"]').text()).toContain('local network access is off in Guardrails')
    w.unmount()
  })

  it('running: the button is disabled and says so', async () => {
    service.nearby.mockResolvedValue(state([], { running: true }))
    const w = await mountSection()
    expect((w.get('[data-test="nearby-check"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(w.get('[data-test="nearby-summary"]').text()).toContain('Looking')
    w.unmount()
  })

  it('Check now starts a look and shows the running state the server answers with', async () => {
    service.nearby.mockResolvedValue(state([dev()]))
    service.nearbyLook.mockResolvedValue(state([dev()], { running: true }))
    const w = await mountSection()
    await w.get('[data-test="nearby-check"]').trigger('click')
    await flushPromises()
    expect(service.nearbyLook).toHaveBeenCalledOnce()
    expect((w.get('[data-test="nearby-check"]').element as HTMLButtonElement).disabled).toBe(true)
    w.unmount()
  })

  it('a refused look shows the server\'s plain message', async () => {
    service.nearby.mockResolvedValue(state([dev()]))
    service.nearbyLook.mockRejectedValue(new Error('The last look was moments ago. Wait a few seconds and try again.'))
    const w = await mountSection()
    await w.get('[data-test="nearby-check"]').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('moments ago')
    w.unmount()
  })

  it('pins a problem above the list and opens the existing guide from its device', async () => {
    service.nearby.mockResolvedValue(state([dev({ names: ['TV'] }), rogue]))
    const w = await mountSection()
    expect(w.get('[data-test="nearby-pinned"]').text()).toContain('not your router')
    await w.get('[data-test="nearby-pinned-fix"]').trigger('click')
    await w.get('[data-test="nearby-fix"]').trigger('click')
    expect(w.text()).toContain('Find the device.')
    w.unmount()
  })

  it('lists a row per device with its name, what it advertises as and its address, problems first', async () => {
    service.nearby.mockResolvedValue(state([dev({ names: ['Office Printer'], services: ['_ipp._tcp'] }), rogue]))
    const w = await mountSection()
    const rows = w.findAll('[data-test="nearby-row"]')
    expect(rows[0]!.text()).toContain('Upstairs box')
    expect(rows[1]!.text()).toContain('Advertises as Printer')
    expect(rows[1]!.text()).toContain('Printing (IPP)')
    // The group heading says whether a device was heard; a row does not say it again.
    for (const row of rows) expect(row.text()).not.toMatch(/heard in the last look/i)
    w.unmount()
  })

  it('a hostile name is text: no element is created and nothing runs', async () => {
    service.nearby.mockResolvedValue(state([dev({ names: [HOSTILE], maker: HOSTILE, model: HOSTILE })]))
    const w = await mountSection()
    await w.get('[data-test="nearby-row"]').trigger('click')
    expect(w.find('img').exists()).toBe(false)
    expect((window as unknown as { __pwned?: number }).__pwned).toBeUndefined()
    expect(w.get('[data-test="nearby-name"]').text()).toContain('<img src=x')
    w.unmount()
  })

  it('an empty filter says so', async () => {
    service.nearby.mockResolvedValue(state([dev()]))
    const w = await mountSection()
    await w.get('[data-test="nearby-filter"]').setValue('attention')
    expect(w.get('[data-test="nearby-no-match"]').text()).toContain('No device matches')
    w.unmount()
  })

  it('Advanced is a table of the raw facts and Copy the results copies a summary without names', async () => {
    useNetauditViewMode().set('advanced')
    service.nearby.mockResolvedValue(state([dev({ names: ['Secret Name'], services: ['_ipp._tcp'], ports: [631], upnp_type: 'Basic:1', sources: ['mdns'] })]))
    const w = await mountSection()
    const table = w.get('[data-test="nearby-table"]').text()
    expect(table).toContain('_ipp._tcp')
    expect(table).toContain('Basic:1')
    expect(table).toContain('631')
    await w.get('[data-test="nearby-copy"]').trigger('click')
    expect(clipboard.copyText.mock.calls[0]![0]).not.toContain('Secret Name')
    w.unmount()
  })

  it('a failed read is an error state with a retry, not an empty list', async () => {
    service.nearby.mockRejectedValue(new Error('boom'))
    const w = await mountSection()
    expect(w.text()).toContain('Nearby could not be read')
    w.unmount()
  })
})

describe('Nearby switch card', () => {
  beforeEach(() => {
    service.nearby.mockReset().mockResolvedValue(state([], { enabled: false }))
    service.setNearby.mockReset().mockResolvedValue(state([], { enabled: true }))
  })

  it('is off until switched on, says what is sent, and saves at once', async () => {
    const w = mount(NearbyCard, { attachTo: document.body })
    await flushPromises()
    expect((w.get('[role="switch"]').element as HTMLInputElement).checked).toBe(false)
    await w.get('[data-test="toggle-details"]').trigger('click')
    const text = w.get('[data-test="toggle-details-text"]').text()
    expect(text).toContain('two questions')
    expect(text).toContain('Nothing else a device sends is stored')
    expect(text).toContain('asks for your passphrase')
    await w.get('[role="switch"]').setValue(true)
    expect(service.setNearby).toHaveBeenCalledWith(true)
    w.unmount()
  })
})
