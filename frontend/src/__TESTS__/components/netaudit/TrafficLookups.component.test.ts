import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import type { DeviceLookupsState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ deviceLookups: vi.fn(), devices: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import TrafficLookups from '../../../components/netaudit/TrafficLookups.vue'

const NOW = '2026-11-11T12:00:00Z'
const state = (over: Partial<DeviceLookupsState> = {}): DeviceLookupsState => ({
  enabled: true, since: '2026-11-01T00:00:00Z',
  devices: [
    { mac: 'aa:aa', first: NOW, last: NOW, today: 2, total: 3, flags: ['known_bad'], names: [
      { name: 'bad.example.net', domain: 'example.net', last: NOW, count: 1, flag: 'known_bad' },
      { name: 'api.apple.com', domain: 'apple.com', last: NOW, count: 9 },
      { name: 'time.apple.com', domain: 'apple.com', last: NOW, count: 2 },
    ] },
    { mac: 'bb:bb', first: NOW, last: NOW, today: 0, total: 0, flags: ['around_dns'], names: [] },
  ],
  ...over,
})
const show = () => mount(TrafficLookups, { global: { stubs: { RouterLink: RouterLinkStub } } })

describe('TrafficLookups', () => {
  beforeEach(() => {
    service.deviceLookups.mockReset()
    service.devices.mockReset()
    service.devices.mockResolvedValue({ devices: [{ mac: 'aa:aa', ip: '192.168.50.10', name: 'Pixel 8', first_seen: NOW, last_seen: NOW, approved: true, randomized: false, new: false }] })
  })

  it('lists each device by the name the person gave it, with its counts and flags, and says it is lookups not traffic', async () => {
    service.deviceLookups.mockResolvedValue(state())
    const w = show()
    await flushPromises()
    const rows = w.findAll('[data-test="lookups-device"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('Pixel 8')
    expect(rows[0]!.text()).toContain('192.168.50.10')
    expect(rows[0]!.text()).toContain('2 names today · 3 kept')
    expect(rows[0]!.get('[data-test="lookups-flags"]').text()).toContain('Known malware host')
    expect(rows[1]!.text()).toContain('Unknown device')
    expect(w.text()).toContain('lookups, not traffic')
  })

  it('opens a device to its names grouped by domain, and shows a silent device\'s reason', async () => {
    service.deviceLookups.mockResolvedValue(state())
    const w = show()
    await flushPromises()
    await w.findAll('[data-test="lookups-toggle"]')[0]!.trigger('click')
    const groups = w.findAll('[data-test="lookups-group"]')
    expect(groups.map((g) => g.text())).toEqual([expect.stringContaining('example.net'), expect.stringContaining('apple.com')])
    expect(groups[1]!.text()).toContain('api.apple.com, time.apple.com')
    expect(groups[0]!.text()).toContain('Known malware host')
    await w.findAll('[data-test="lookups-toggle"]')[1]!.trigger('click')
    expect(w.get('[data-test="lookups-names"]').text()).toContain('resolver of its own')
  })

  it('shows names as text, never as markup', async () => {
    const evil = '<img src=x onerror=alert(1)>.example.com'
    service.deviceLookups.mockResolvedValue(state({ devices: [{ mac: 'aa:aa', first: NOW, last: NOW, today: 1, total: 1, flags: [], names: [{ name: evil, domain: 'example.com', last: NOW, count: 1 }] }] }))
    const w = show()
    await flushPromises()
    await w.get('[data-test="lookups-toggle"]').trigger('click')
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain(evil)
  })

  it('says the watch is off, with a link to its switch, instead of an empty list', async () => {
    service.deviceLookups.mockResolvedValue({ enabled: false, devices: [] })
    const w = show()
    await flushPromises()
    expect(w.get('[data-test="lookups-off"]').text()).toContain('Phone-home watch is off')
    expect(w.findComponent(RouterLinkStub).exists()).toBe(true)
    expect(w.find('[data-test="lookups-empty"]').exists()).toBe(false)
  })

  it('says when nothing has been asked yet, and when the lookups could not be read', async () => {
    service.deviceLookups.mockResolvedValue({ enabled: true, devices: [] })
    const w = show()
    await flushPromises()
    expect(w.get('[data-test="lookups-empty"]').text()).toContain('No device has asked')
    service.deviceLookups.mockRejectedValue(new Error(''))
    await w.get('[data-test="lookups-refresh"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="lookups-error"]').text()).toContain('could not be read')
  })
})
