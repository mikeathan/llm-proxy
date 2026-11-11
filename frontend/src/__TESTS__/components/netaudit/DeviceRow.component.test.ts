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

describe('DeviceRow problems link', () => {
  const withProblems = async (lookup: { of: (a: string) => { count: number; severity: string } | null } | null, d = device()) => {
    const { createMemoryHistory } = await import('vue-router')
    const { createAppRouter } = await import('../../../router')
    const { DEVICE_PROBLEMS_INJECTION } = await import('../../../composables/netaudit/useDeviceProblems')
    return mount(DeviceRow, {
      props: { device: d, expanded: false, editing: false, draft: '', busy: false, advanced: false },
      global: { plugins: [createAppRouter(createMemoryHistory())], provide: lookup ? { [DEVICE_PROBLEMS_INJECTION as symbol]: lookup } : {} },
    })
  }

  it('links a device that has open problems to them, beside the row and not inside its button', async () => {
    const w = await withProblems({ of: (a) => (a === '192.0.2.5' ? { count: 2, severity: 'high' } : null) })
    const link = w.get('[data-test="device-problems"]')
    expect(link.attributes('href')).toBe('/network/findings?device=192.0.2.5')
    expect(link.text()).toBe('2 problems')
    expect(link.attributes('aria-label')).toContain('Show the 2 problems found on Chromecast')
    expect(w.get('[data-test="device-toggle"]').find('[data-test="device-problems"]').exists()).toBe(false)
  })

  it('says "1 problem" for one, and shows nothing for a device without problems or without a lookup', async () => {
    expect((await withProblems({ of: () => ({ count: 1, severity: 'low' }) })).get('[data-test="device-problems"]').text()).toBe('1 problem')
    expect((await withProblems({ of: () => null })).find('[data-test="device-problems"]').exists()).toBe(false)
    expect((await withProblems(null)).find('[data-test="device-problems"]').exists()).toBe(false)
  })

  it('has no link for a device that has no IPv4 address to match the audit by', async () => {
    const w = await withProblems({ of: () => ({ count: 1, severity: 'low' }) }, device({ ip: undefined }))
    expect(w.find('[data-test="device-problems"]').exists()).toBe(false)
  })
})
