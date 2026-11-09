import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TrafficReport from '../../../components/netaudit/TrafficReport.vue'
import type { HostTraffic } from '../../../types/netaudit'

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
})
