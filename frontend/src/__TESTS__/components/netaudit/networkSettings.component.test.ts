import { describe, expect, it } from 'vitest'
import { shallowMount } from '@vue/test-utils'

import NetworkSettings from '../../../components/netaudit/NetworkSettings.vue'

// The Settings tab is a composition: what matters here is which parts it holds, under which heading, in which order.
describe('NetworkSettings', () => {
  const mounted = () => shallowMount(NetworkSettings)
  const order = (w: ReturnType<typeof mounted>) => w.findAll('[data-test="network-settings"] > *').map((el) => el.attributes('data-test') ?? el.element.tagName.toLowerCase())

  it('puts what may be scanned first and the Advanced limits last, with the vulnerability data before them', () => {
    const html = mounted().html()
    const at = (needle: string) => html.indexOf(needle)
    expect(at('scan-scope-settings-stub')).toBeGreaterThan(-1)
    expect(at('scan-scope-settings-stub')).toBeLessThan(at('data-test="group-watching"'))
    expect(at('data-test="group-watching"')).toBeLessThan(at('data-test="group-extras"'))
    expect(at('data-test="group-extras"')).toBeLessThan(at('vuln-data-card-stub'))
    expect(at('vuln-data-card-stub')).toBeLessThan(at('scan-limits-settings-stub'))
  })

  it('groups the checks under Watching for changes, and the looks that go further under Optional extras', () => {
    const w = mounted()
    const watching = w.get('[data-test="group-watching"]')
    expect(watching.get('h2').text()).toBe('Watching for changes')
    expect(watching.html()).toContain('device-check-settings-stub')
    // The weekly summary and the daily check-in are switches, and every switch is on the Modules tab: nothing here switches them.
    expect(watching.html()).not.toContain('Regular messages')
    expect(w.find('[role="switch"]').exists()).toBe(false)
    const extras = w.get('[data-test="group-extras"]')
    expect(extras.get('h2').text()).toBe('Optional extras')
    for (const card of ['decoys-card-stub', 'router-log-card-stub', 'outside-lookup-card-stub', 'dns-check-card-stub', 'nearby-card-stub']) expect(extras.html()).toContain(card)
  })

  it('says that every change saves at once, and has no Save button of its own', () => {
    const w = mounted()
    expect(w.text()).toContain('Every change saves at once.')
    expect(order(w)[0]).toBe('p')
  })
})
