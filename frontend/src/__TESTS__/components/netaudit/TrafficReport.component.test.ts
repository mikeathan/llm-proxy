import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TrafficReport from '../../../components/netaudit/TrafficReport.vue'
import type { HostTraffic } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ devices: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

const MB = 1024 * 1024
const traffic = (over: Partial<HostTraffic> = {}): HostTraffic => ({
  at: '2026-11-09T20:00:00Z', interface: 'wlp2s0', stopped_by: 'window', elapsed_ms: 60_000, frames: 100, unreadable: 0, kernel_seen: 100, kernel_dropped: 0, sample: 1,
  bytes_in: 38 * MB, bytes_out: 4 * MB, bytes_through: 0, internet_bytes: 41 * MB, local_network_bytes: MB, lookups: 212,
  kinds: [{ kind: 'https', bytes: 40 * MB }, { kind: 'dns', bytes: MB }, { kind: 'udp', bytes: MB }],
  per_second: [100, 5000, 200],
  destinations: [
    { name: 'generativelanguage.googleapis.com', address: '142.250.1.1', bytes: 22 * MB, internet: true },
    { name: 'github.com', address: '140.82.1.1', bytes: 10 * MB, internet: true },
    { address: '34.120.1.1', bytes: 6 * MB, internet: true },
  ],
  flows: [
    { address: '142.250.1.1', name: 'generativelanguage.googleapis.com', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 2 * MB, bytes_in: 20 * MB },
    { address: '192.168.50.1', port: 53, proto: 'udp', kind: 'dns', internet: false, bytes_out: 1000, bytes_in: 4000 },
  ],
  other_flows: 3, other_bytes: 2 * MB, ...over,
})
const show = (t: HostTraffic) => mount(TrafficReport, { props: { traffic: t } })

describe('TrafficReport', () => {
  beforeEach(() => {
    service.devices.mockReset()
    service.devices.mockResolvedValue({ devices: [] })
  })

  it('opens with the one sentence a person can read, and the details under it', () => {
    const w = show(traffic())
    expect(w.get('[data-test="traffic-headline"]').text()).toBe('In 60 seconds this computer sent 4 MB and received 38 MB.')
    expect(w.get('[data-test="traffic-detail"]').text()).toContain('3 destinations took')
  })

  it('ranks where the data went, by name when there is one and by address with a plain note when there is not', () => {
    const rows = show(traffic()).findAll('[data-test="destination"]')
    expect(rows.map((r) => r.get('[data-test="destination-name"]').text())).toEqual(['generativelanguage.googleapis.com', 'github.com', '34.120.1.1'])
    expect(rows[2]!.text()).toContain('no name seen')
    expect(rows[0]!.text()).toContain('22 MB')
  })

  it('counts what is not in the top places as "other" so the list adds up', () => {
    const w = show(traffic())
    expect(w.get('[data-test="destinations-other"]').text()).toContain('3 other places')
  })

  it('shows the kinds of traffic as shares that read as words, and the way it went', () => {
    const w = show(traffic())
    const kinds = w.findAll('[data-test="kind"]').map((k) => k.text())
    expect(kinds[0]).toContain('Web, encrypted (HTTPS)')
    expect(kinds[0]).toContain('95%')
    expect(w.get('[data-test="traffic-split"]').text()).toContain('received')
  })

  it('gives the four totals in the summary, beside the chart', () => {
    const w = show(traffic())
    expect(w.get('[data-test="stat-sent"]').text()).toBe('4 MB')
    expect(w.get('[data-test="stat-received"]').text()).toBe('38 MB')
    expect(w.get('[data-test="stat-internet"]').text()).toBe('41 MB')
    expect(w.get('[data-test="stat-network"]').text()).toBe('1 MB')
  })

  it('draws the seconds as a chart that has a text name for a screen reader', () => {
    const w = show(traffic())
    const chart = w.get('[data-test="traffic-chart"]')
    expect(chart.attributes('role')).toBe('img')
    expect(chart.attributes('aria-label')).toContain('busiest second')
    expect(chart.findAll('rect')).toHaveLength(3)
  })

  it('lists the conversations with their kind, port and both directions', () => {
    const rows = show(traffic()).findAll('[data-test="flow"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('443')
    expect(rows[0]!.text()).toContain('HTTPS')
    expect(rows[0]!.text()).toContain('20 MB')
    expect(rows[1]!.text()).toContain('192.168.50.1')
  })

  it('says when the numbers are estimates, partial, or lost frames, and says nothing for a clean window', () => {
    expect(show(traffic()).find('[data-test="traffic-notes"]').exists()).toBe(false)
    const w = show(traffic({ sample: 8, stopped_by: 'frame_limit', kernel_dropped: 4 }))
    const notes = w.get('[data-test="traffic-notes"]').text()
    expect(notes).toContain('Estimated: one ordinary frame in 8')
    expect(notes).toContain('limit')
    expect(notes).toContain('4 frames were dropped')
  })

  it('handles a window with nothing in it without drawing empty boxes', () => {
    const w = show(traffic({ bytes_in: 0, bytes_out: 0, internet_bytes: 0, local_network_bytes: 0, kinds: [], destinations: [], flows: [], per_second: [], other_flows: 0, other_bytes: 0, lookups: 0 }))
    expect(w.get('[data-test="traffic-headline"]').text()).toContain('nothing was sent or received')
    expect(w.find('[data-test="destination"]').exists()).toBe(false)
    expect(w.find('[data-test="traffic-chart"]').exists()).toBe(false)
    expect(w.find('[data-test="flow"]').exists()).toBe(false)
  })

  it('always says what the view can and cannot see', () => {
    expect(show(traffic()).get('[data-test="traffic-privacy"]').text()).toContain('only the traffic to and from this computer')
  })

  describe('who runs the servers', () => {
    const owned = () => traffic({
      owners_used: true,
      owners: [{ asn: 15169, name: 'GOOGLE - Google LLC', country: 'US', bytes: 22 * MB }, { asn: 36459, name: 'GITHUB', country: 'US', bytes: 10 * MB }],
      countries: [{ country: 'US', bytes: 32 * MB }, { country: 'DE', bytes: 4 * MB }],
      destinations: [
        { name: 'generativelanguage.googleapis.com', name_from: 'tls', address: '142.250.1.1', bytes: 22 * MB, internet: true, owner: { asn: 15169, name: 'GOOGLE - Google LLC', country: 'US' } },
        { address: '34.120.1.1', bytes: 6 * MB, internet: true },
      ],
      flows: [{ address: '142.250.1.1', name: 'generativelanguage.googleapis.com', name_from: 'tls', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 2 * MB, bytes_in: 20 * MB, owner: { asn: 15169, name: 'GOOGLE - Google LLC', country: 'US' } }],
    })

    it('puts the operator and country under a destination and a conversation that has an owner, and nothing under one that has none', () => {
      const w = show(owned())
      const rows = w.findAll('[data-test="destination"]')
      expect(rows[0]!.get('[data-test="destination-owner"]').text()).toBe('GOOGLE - Google LLC · United States')
      expect(rows[1]!.find('[data-test="destination-owner"]').exists()).toBe(false)
      expect(w.get('[data-test="flow-owner"]').text()).toContain('United States')
    })

    const pick = async (w: ReturnType<typeof show>, label: string) => { await w.findAll('[role="radio"]').find((b) => b.text() === label)!.trigger('click') }

    it('ranks the internet traffic by operator and by country, as shares of the internet traffic', async () => {
      const w = show(owned())
      await pick(w, 'Operators')
      const owners = w.get('[data-test="where-operators"]').findAll('[data-test="where-row"]')
      expect(owners.map((r) => r.get('[data-test="where-label"]').text())).toEqual(['GOOGLE - Google LLC AS15169 · United States', 'GITHUB AS36459 · United States'])
      expect(owners[0]!.text()).toContain('54%')
      await pick(w, 'Countries')
      const countries = w.get('[data-test="where-countries"]').findAll('[data-test="where-row"]')
      expect(countries[0]!.get('[data-test="where-label"]').text()).toContain('United States')
      expect(countries[1]!.get('[data-test="where-label"]').text()).toContain('Germany')
    })

    it('says how to get owners when the look was made without the table, and never guesses', async () => {
      const w = show(traffic())
      await pick(w, 'Operators')
      expect(w.get('[data-test="where-no-table"]').text()).toContain('without the table')
      expect(w.find('[data-test="where"]').exists()).toBe(false)
    })

    it('says so when the table is held and none of the addresses is in it', async () => {
      const w = show(traffic({ owners_used: true, owners: [], countries: [] }))
      await pick(w, 'Operators')
      expect(w.find('[data-test="where-nothing"]').exists()).toBe(true)
    })

    it('offers no operator or country view when nothing went to the internet', () => {
      const w = show(traffic({ internet_bytes: 0, owners_used: true, owners: [] }))
      expect(w.findAll('[role="radio"]')).toHaveLength(0)
      expect(w.find('[data-test="where-no-table"]').exists()).toBe(false)
    })

    it('treats a holder\'s name as text, never as markup', async () => {
      const evil = '<img src=x onerror=alert(1)>'
      const w = show(traffic({ owners_used: true, owners: [{ asn: 1, name: evil, country: 'US', bytes: MB }], countries: [{ country: 'US', bytes: MB }] }))
      await pick(w, 'Operators')
      expect(w.find('img').exists()).toBe(false)
      expect(w.get('[data-test="where-operators"]').text()).toContain(evil)
    })

    it('says where a place\'s name came from and who runs it, in its detail', async () => {
      const w = mount(TrafficReport, { props: { traffic: owned() }, global: { stubs: { DeviceProbe: true } } })
      await w.findAll('[data-test="destination-open"]')[0]!.trigger('click')
      expect(w.get('[data-test="place-owner"]').text()).toContain('GOOGLE - Google LLC')
      expect(w.get('[data-test="place-owner"]').text()).toContain('AS15169')
      expect(w.get('[data-test="place-name-from"]').text()).toContain('TLS handshake')
    })
  })

  describe('clicking through', () => {
    const withLocal = () => traffic({
      destinations: [
        { name: 'github.com', address: '140.82.1.1', bytes: 10 * MB, internet: true },
        { address: '192.168.50.7', bytes: 2 * MB, internet: false },
        { address: '224.0.0.251', bytes: 1000, internet: false },
      ],
      flows: [
        { address: '140.82.1.1', name: 'github.com', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: MB, bytes_in: 9 * MB },
        { address: '192.168.50.7', port: 53, proto: 'udp', kind: 'dns', internet: false, bytes_out: 100, bytes_in: 200 },
        { address: '224.0.0.251', port: 5353, proto: 'udp', kind: 'discovery', internet: false, bytes_out: 1000, bytes_in: 0 },
      ],
    })
    const stubs = { DeviceProbe: { props: ['ip', 'mac', 'udpAt'], template: '<div data-test="probe-stub">{{ ip }}|{{ mac }}|{{ udpAt }}</div>' } }
    const open = (t: HostTraffic) => mount(TrafficReport, { props: { traffic: t }, global: { stubs } })

    it('opens a place from the ranking, with its size, share and conversations, and says an internet address is not scanned', async () => {
      const w = open(withLocal())
      expect(w.find('[data-test="traffic-detail-panel"]').exists()).toBe(false)
      await w.findAll('[data-test="destination-open"]')[0]!.trigger('click')
      expect(w.get('[data-test="place-address"]').text()).toBe('140.82.1.1')
      expect(w.get('[data-test="place-where"]').text()).toBe('On the internet')
      expect(w.findAll('[data-test="place-flow"]')).toHaveLength(1)
      expect(w.find('[data-test="place-read-only"]').exists()).toBe(true)
      expect(w.find('[data-test="probe-stub"]').exists()).toBe(false)
    })

    it('opens a place from a conversation row, and offers the device scans for a device on this network', async () => {
      const w = open(withLocal())
      await w.findAll('[data-test="flow"]')[1]!.trigger('click')
      expect(w.get('[data-test="place-where"]').text()).toBe('On this network')
      expect(w.get('[data-test="probe-stub"]').text()).toBe('192.168.50.7||')
    })

    it('gives the scan buttons the device\'s MAC and its kept scan, so the clear buttons show', async () => {
      service.devices.mockResolvedValue({ devices: [{ mac: 'aa:bb:cc:dd:ee:07', ip: '192.168.50.7', udp_scanned_at: '2026-11-09T19:00:00Z' }] })
      const w = open(withLocal())
      await w.findAll('[data-test="flow"]')[1]!.trigger('click')
      await flushPromises()
      expect(w.get('[data-test="probe-stub"]').text()).toBe('192.168.50.7|aa:bb:cc:dd:ee:07|2026-11-09T19:00:00Z')
    })

    it('still offers the scans when the device list cannot be read', async () => {
      service.devices.mockRejectedValue(new Error('down'))
      const w = open(withLocal())
      await w.findAll('[data-test="flow"]')[1]!.trigger('click')
      await flushPromises()
      expect(w.get('[data-test="probe-stub"]').text()).toBe('192.168.50.7||')
    })

    it('never offers a scan for a multicast address, and says why', async () => {
      const w = open(withLocal())
      await w.findAll('[data-test="flow"]')[2]!.trigger('click')
      expect(w.find('[data-test="probe-stub"]').exists()).toBe(false)
      expect(w.find('[data-test="place-shared"]').exists()).toBe(true)
    })

    it('closes the detail, and drops it when a new look arrives', async () => {
      const w = open(withLocal())
      await w.findAll('[data-test="destination-open"]')[0]!.trigger('click')
      await w.get('[data-test="traffic-detail-close"]').trigger('click')
      expect(w.find('[data-test="traffic-detail-panel"]').exists()).toBe(false)
      await w.findAll('[data-test="destination-open"]')[0]!.trigger('click')
      await w.setProps({ traffic: withLocal() })
      expect(w.find('[data-test="traffic-detail-panel"]').exists()).toBe(false)
    })

    it('filters the conversations to one kind, and back', async () => {
      const w = open(withLocal())
      expect(w.findAll('[data-test="flow"]')).toHaveLength(3)
      await w.findAll('[data-test="kind-pick"]')[1]!.trigger('click')
      expect(w.findAll('[data-test="kind-pick"]')[1]!.attributes('aria-pressed')).toBe('true')
      expect(w.findAll('[data-test="flow"]')).toHaveLength(1)
      expect(w.get('[data-test="kind-filter"]').text()).toContain('Showing only')
      await w.get('[data-test="kind-filter-clear"]').trigger('click')
      expect(w.findAll('[data-test="flow"]')).toHaveLength(3)
    })

    it('says so when a kind has no conversation kept, instead of an empty table', async () => {
      const w = open(withLocal())
      await w.findAll('[data-test="kind-pick"]')[2]!.trigger('click')
      expect(w.find('[data-test="flows-none"]').exists()).toBe(true)
      expect(w.find('[data-test="flows"]').exists()).toBe(false)
    })

    it('reads out the bytes of a second picked on the chart', async () => {
      const w = open(withLocal())
      expect(w.find('[data-test="traffic-second"]').exists()).toBe(false)
      await w.get('[data-test="traffic-second-pick"]').setValue('1')
      expect(w.get('[data-test="traffic-second"]').text()).toBe('0:01: 4.9 KB in this second')
    })
  })
})

describe('TrafficReport problems link', () => {
  const lan = () => traffic({
    destinations: [{ address: '192.168.50.7', bytes: 2 * MB, internet: false }, { name: 'github.com', address: '140.82.1.1', bytes: MB, internet: true }],
    flows: [{ address: '192.168.50.7', port: 53, proto: 'udp', kind: 'dns', internet: false, bytes_out: 100, bytes_in: 200 }],
  })
  const open = async (lookup: { of: (a: string) => { count: number; severity: string } | null }) => {
    const { createMemoryHistory } = await import('vue-router')
    const { createAppRouter } = await import('../../../router')
    const { DEVICE_PROBLEMS_INJECTION } = await import('../../../composables/netaudit/useDeviceProblems')
    return mount(TrafficReport, { props: { traffic: lan() }, global: { stubs: { DeviceProbe: true }, plugins: [createAppRouter(createMemoryHistory())], provide: { [DEVICE_PROBLEMS_INJECTION as symbol]: lookup } } })
  }

  it('links a device on this network that has open problems to them', async () => {
    const w = await open({ of: (a) => (a === '192.168.50.7' ? { count: 3, severity: 'high' } : null) })
    await w.get('[data-test="places-network"] [data-test="destination-open"]').trigger('click')
    const link = w.get('[data-test="place-problems"] a')
    expect(link.attributes('href')).toBe('/network/findings?device=192.168.50.7')
    expect(link.text()).toBe('3 problems found on this device')
  })

  it('offers no link for an internet address or a device without problems', async () => {
    const w = await open({ of: () => ({ count: 1, severity: 'low' }) })
    await w.get('[data-test="places-internet"] [data-test="destination-open"]').trigger('click')
    expect(w.find('[data-test="place-problems"]').exists()).toBe(false)
    const none = await open({ of: () => null })
    await none.get('[data-test="places-network"] [data-test="destination-open"]').trigger('click')
    expect(none.find('[data-test="place-problems"]').exists()).toBe(false)
  })
})

describe('TrafficReport places in groups', () => {
  const mixed = () => traffic({
    owners_used: true,
    destinations: [
      { address: '192.168.50.251', bytes: 5 * MB, internet: false },
      { name: 'github.com', address: '140.82.1.1', bytes: 3 * MB, internet: true, owner: { asn: 36459, name: 'GITHUB', country: 'US' } },
      { address: '224.0.0.251', bytes: 4000, internet: false },
      { address: '192.168.50.1', bytes: MB, internet: false },
    ],
    flows: [
      { address: '140.82.1.1', name: 'github.com', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: MB, bytes_in: 2 * MB },
      { address: '140.82.1.1', name: 'github.com', port: 22, proto: 'tcp', kind: 'ssh', internet: true, bytes_out: 1000, bytes_in: 1000 },
    ],
  })

  it('lists the internet first, then this network, then the shared addresses, each biggest first', () => {
    const w = show(mixed())
    const names = (id: string) => w.get(`[data-test="places-${id}"]`).findAll('[data-test="destination-name"]').map((n) => n.text())
    expect(names('internet')).toEqual(['github.com'])
    expect(names('network')).toEqual(['192.168.50.251', '192.168.50.1'])
    expect(names('shared')).toEqual(['224.0.0.251'])
    expect(w.get('[data-test="places-internet"] [data-test="destination-owner"]').text()).toBe('GITHUB · United States')
    expect(w.get('[data-test="places-internet"] [data-test="destination-kinds"]').text()).toBe('HTTPS, SSH')
  })

  it('names a device on this network the way the person does', async () => {
    service.devices.mockResolvedValue({ devices: [{ mac: 'aa:bb:cc:dd:ee:01', ip: '192.168.50.1', name: 'Router' }] })
    const w = show(mixed())
    await flushPromises()
    expect(w.get('[data-test="places-network"] [data-test="destination-device"]').text()).toContain('Router')
  })

  it('shows the first conversations and the rest on request', async () => {
    const many = Array.from({ length: 15 }, (_, i) => ({ address: `10.0.0.${i + 1}`, port: 80, proto: 'tcp' as const, kind: 'http' as const, internet: false, bytes_out: 10, bytes_in: 10 }))
    const w = show(traffic({ flows: many }))
    expect(w.findAll('[data-test="flow"]')).toHaveLength(12)
    await w.get('[data-test="flows-toggle"]').trigger('click')
    expect(w.findAll('[data-test="flow"]')).toHaveLength(15)
  })
})
