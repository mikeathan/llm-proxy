import { describe, expect, it } from 'vitest'
import { RouterLinkStub, mount } from '@vue/test-utils'
import TrafficDeviceNames from '../../../components/netaudit/TrafficDeviceNames.vue'
import type { DeviceLookups } from '../../../types/netaudit'

const NOW = '2026-11-11T12:00:00Z'
const lookups = (over: Partial<DeviceLookups> = {}): DeviceLookups => ({
  mac: 'aa:aa', first: NOW, last: NOW, today: 2, total: 3, flags: ['known_bad'],
  names: [
    { name: 'bad.example.net', domain: 'example.net', last: NOW, count: 1, flag: 'known_bad' },
    { name: 'api.apple.com', domain: 'apple.com', last: NOW, count: 9 },
    { name: 'time.apple.com', domain: 'apple.com', last: NOW, count: 2 },
  ],
  ...over,
})
const show = (props: Partial<InstanceType<typeof TrafficDeviceNames>['$props']> = {}) =>
  mount(TrafficDeviceNames, { props: { lookups: lookups(), enabled: true, known: true, ...props }, global: { stubs: { RouterLink: RouterLinkStub } } })

describe('TrafficDeviceNames', () => {
  it('shows what a device asked for, grouped by domain, with its counts and flags, and says it is lookups not traffic', () => {
    const w = show()
    expect(w.get('[data-test="names-counts"]').text()).toContain('2 names today · 3 kept')
    expect(w.get('[data-test="names-flags"]').text()).toContain('Known malware host')
    const groups = w.findAll('[data-test="names-group"]')
    expect(groups.map((g) => g.text())).toEqual([expect.stringContaining('example.net'), expect.stringContaining('apple.com')])
    expect(groups[1]!.text()).toContain('api.apple.com, time.apple.com')
    expect(w.text()).toContain('Lookups, not traffic')
  })

  it('gives the reason for a device that asked for nothing', () => {
    expect(show({ lookups: lookups({ names: [], flags: ['around_dns'] }) }).get('[data-test="names-none"]').text()).toContain('resolver of its own')
  })

  it('shows names as text, never as markup', () => {
    const evil = '<img src=x onerror=alert(1)>.example.com'
    const w = show({ lookups: lookups({ names: [{ name: evil, domain: 'example.com', last: NOW, count: 1 }], flags: [] }) })
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain(evil)
  })

  it('says the watch is off with a link to its switch, and that an unlisted device has no names yet', () => {
    const off = show({ enabled: false, lookups: null })
    expect(off.get('[data-test="names-off"]').text()).toContain('Phone-home watch is off')
    expect(off.findComponent(RouterLinkStub).props('to')).toEqual({ name: 'network', params: { tab: 'settings' }, query: { group: 'outside' } })
    expect(show({ known: false, lookups: null }).get('[data-test="names-unknown"]').text()).toContain('Devices list')
  })

  it('says when the lookups could not be read, and cuts a long list with a way to see all', async () => {
    expect(show({ error: 'The lookups could not be read.' }).get('[data-test="names-error"]').text()).toContain('could not be read')
    const many = Array.from({ length: 9 }, (_, i) => ({ name: `a.d${i}.com`, domain: `d${i}.com`, last: NOW, count: 1 }))
    const w = show({ lookups: lookups({ names: many, flags: [] }) })
    expect(w.findAll('[data-test="names-group"]')).toHaveLength(6)
    await w.get('[data-test="names-more"]').trigger('click')
    expect(w.findAll('[data-test="names-group"]')).toHaveLength(9)
  })
})
