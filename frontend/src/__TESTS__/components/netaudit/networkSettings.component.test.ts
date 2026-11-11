import { describe, expect, it } from 'vitest'
import { RouterLinkStub, shallowMount } from '@vue/test-utils'

import NetworkSettings from '../../../components/netaudit/NetworkSettings.vue'
import SettingsGroupNav from '../../../components/netaudit/SettingsGroupNav.vue'
import { NETWORK_SETTINGS_GROUPS } from '../../../types/routes'
import type { NetworkSettingsGroup } from '../../../types/routes'

// The Settings tab is a composition: what matters here is which group shows which cards. A group is one job; every card is in exactly one group.
const GROUP_CARDS: Readonly<Record<NetworkSettingsGroup, readonly string[]>> = {
  scanning: ['scan-scope-settings-stub', 'scan-limits-settings-stub'],
  watching: ['device-check-settings-stub', 'decoys-card-stub', 'router-log-card-stub', 'dns-check-card-stub', 'nearby-card-stub', 'wifi-card-stub'],
  outside: ['outside-lookup-card-stub', 'footprint-card-stub', 'known-bad-card-stub'],
  data: ['vuln-data-card-stub', 'ubuntu-data-card-stub'],
  helper: ['helper-card-stub'],
}

describe('NetworkSettings', () => {
  const mounted = (group: NetworkSettingsGroup) => shallowMount(NetworkSettings, { props: { group }, global: { stubs: { SettingsGroupNav: true } } })

  it.each(NETWORK_SETTINGS_GROUPS)('shows the cards of the %s group and no others', (group) => {
    const html = mounted(group).get(`[data-test="group-${group}"]`).html()
    for (const card of GROUP_CARDS[group]) expect(html).toContain(card)
    for (const other of NETWORK_SETTINGS_GROUPS.filter((g) => g !== group)) for (const card of GROUP_CARDS[other]) expect(html).not.toContain(card)
  })

  it('puts what may be scanned first, so a link without a group opens it', () => {
    expect(NETWORK_SETTINGS_GROUPS[0]).toBe('scanning')
  })

  it('says what the open group holds and that every change saves at once, with no Save button of its own', () => {
    const w = mounted('outside')
    expect(w.get('[data-test="settings-hint"]').text()).toContain('Every change saves at once.')
    expect(w.get('[data-test="settings-hint"]').text()).toContain('ask a service on the internet')
    expect(w.find('[role="switch"]').exists()).toBe(false)
  })
})

describe('SettingsGroupNav', () => {
  it('links to each group and marks the open one as the current page', () => {
    const w = shallowMount(SettingsGroupNav, { props: { active: 'data' }, global: { stubs: { RouterLink: RouterLinkStub, ScrollStrip: { template: '<nav><slot /></nav>' } } } })
    const links = w.findAllComponents(RouterLinkStub)
    expect(links.map((l) => (l.props('to') as { query: { group: string } }).query.group)).toEqual([...NETWORK_SETTINGS_GROUPS])
    expect(w.findAll('[aria-current="page"]').map((el) => el.text())).toEqual(['Downloaded data'])
  })
})
