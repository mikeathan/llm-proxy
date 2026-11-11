import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TrafficHeadline from '../../../components/netaudit/TrafficHeadline.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import type { HostTraffic } from '../../../types/netaudit'

const MB = 1024 * 1024
const traffic = (over: Partial<HostTraffic> = {}): HostTraffic => ({
  at: '2026-11-09T20:00:00Z', interface: 'wlp2s0', stopped_by: 'window', elapsed_ms: 60_000, frames: 100, unreadable: 0, kernel_seen: 100, kernel_dropped: 0, sample: 1,
  bytes_in: 38 * MB, bytes_out: 4 * MB, bytes_through: 0, internet_bytes: 41 * MB, local_network_bytes: MB, lookups: 212,
  kinds: [{ kind: 'https', bytes: 40 * MB }, { kind: 'dns', bytes: MB }, { kind: 'udp', bytes: MB }],
  per_second: [100, 5000, 200], destinations: [], flows: [], other_flows: 0, other_bytes: 0, ...over,
})
const show = (t: HostTraffic) => mount(TrafficHeadline, { props: { traffic: t } })

describe('TrafficHeadline', () => {
  beforeEach(() => { useNetauditViewMode().set('simple') })

  it('opens with one sentence, where it went and how encrypted it was', () => {
    const w = show(traffic())
    expect(w.get('[data-test="traffic-headline"]').text()).toBe('In 60 seconds this computer sent 4 MB and received 38 MB.')
    expect(w.get('[data-test="traffic-detail"]').text()).toContain('41 MB to the internet, 1 MB on this network.')
    expect(w.get('[data-test="traffic-only-this"]').text()).toContain('your phone and your other computers are not')
  })

  it('says it could not tell which way traffic went for a look kept before the fix, never that nothing was sent', () => {
    const w = show(traffic({ bytes_in: 0, bytes_out: 0, bytes_through: 1_664_770 }))
    expect(w.get('[data-test="traffic-headline"]').text()).toContain('could not tell which way it went')
  })

  it('sets this program\'s own page apart', () => {
    expect(show(traffic({ self_bytes: 21 * MB })).get('[data-test="traffic-self"]').text()).toContain('this program\'s own page and API')
    expect(show(traffic()).find('[data-test="traffic-self"]').exists()).toBe(false)
  })

  it('keeps the totals and the chart for Advanced', async () => {
    const w = show(traffic())
    expect(w.find('[data-test="traffic-stats"]').exists()).toBe(false)
    expect(w.find('[data-test="traffic-chart"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    await flushPromises()
    expect(w.get('[data-test="stat-sent"]').text()).toBe('4 MB')
    expect(w.get('[data-test="traffic-chart"]').attributes('aria-label')).toContain('busiest second')
  })

  it('says when the numbers are estimates, partial, or lost frames, and nothing for a clean window', () => {
    expect(show(traffic()).find('[data-test="traffic-notes"]').exists()).toBe(false)
    const notes = show(traffic({ sample: 8, stopped_by: 'frame_limit', kernel_dropped: 4 })).get('[data-test="traffic-notes"]').text()
    expect(notes).toContain('Estimated: one ordinary frame in 8')
    expect(notes).toContain('4 frames were dropped')
  })
})
