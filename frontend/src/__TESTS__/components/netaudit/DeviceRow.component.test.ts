import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DeviceRow from '../../../components/netaudit/DeviceRow.vue'
import type { NetworkDevice } from '../../../types/netaudit'

const device = (over: Partial<NetworkDevice> = {}): NetworkDevice => ({
  mac: 'aa:bb:cc:00:00:01', ip: '192.0.2.5', name: 'Chromecast', first_seen: '2026-11-01T00:00:00Z', last_seen: new Date().toISOString(), approved: true, randomized: false, new: false, ...over,
})
const row = (d: NetworkDevice, expanded = false) => mount(DeviceRow, { props: { device: d, expanded, editing: false, draft: '', busy: false, advanced: false } })

// A device the router never hears ask for a name is a note on its row, not a finding to accept (vertex, 2026-11-09: nine of them).
describe('DeviceRow DNS note', () => {
  it('shows a quiet tag on the row and the harmless reasons once opened, only for a silent device', () => {
    const silent = row(device({ dns_silent: true }), true)
    expect(silent.get('[data-test="device-dns-tag"]').text()).toBe('Not heard on router DNS')
    expect(silent.get('[data-test="device-dns-note"]').text()).toContain('Private DNS')
    const normal = row(device(), true)
    expect(normal.find('[data-test="device-dns-tag"]').exists()).toBe(false)
    expect(normal.find('[data-test="device-dns-note"]').exists()).toBe(false)
  })

  it('keeps the explanation out of a folded row', () => {
    const folded = row(device({ dns_silent: true }), false)
    expect(folded.find('[data-test="device-dns-tag"]').exists()).toBe(true)
    expect(folded.find('[data-test="device-dns-note"]').exists()).toBe(false)
  })
})
