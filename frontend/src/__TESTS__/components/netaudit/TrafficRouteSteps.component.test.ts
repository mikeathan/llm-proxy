import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TrafficRouteSteps from '../../../components/netaudit/TrafficRouteSteps.vue'
import type { RouteTrace } from '../../../types/netaudit'

const silent = (ttl: number) => ({ ttl, sent: 3, answered: 0 })
const trace = (over: Partial<RouteTrace> = {}): RouteTrace => ({
  address: '91.189.91.61', state: 'done', started_at: '2026-11-11T14:00:00Z', reached: true,
  hops: [{ ttl: 1, address: '192.168.50.1', sent: 3, answered: 3, best_ms: 1.6, where: 'this_network' }, silent(2), silent(3), silent(4), { ttl: 5, address: '91.189.91.61', sent: 3, answered: 2, best_ms: 97, where: 'internet', reached: true }],
  ...over,
})

describe('TrafficRouteSteps', () => {
  it('folds the silent run into one line and unfolds it on request', async () => {
    const w = mount(TrafficRouteSteps, { props: { trace: trace() } })
    expect(w.findAll('[data-test="route-hop"]')).toHaveLength(2)
    expect(w.get('[data-test="route-silent-run"]').text()).toContain('3 steps with no answer')
    expect(w.get('[data-test="route-summary"]').text()).toContain('5 steps, 2 answered')
    await w.get('[data-test="route-fold-toggle"]').trigger('click')
    expect(w.findAll('[data-test="route-hop"]')).toHaveLength(5)
    expect(w.find('[data-test="route-silent-run"]').exists()).toBe(false)
    expect(w.get('[data-test="route-outcome"]').text()).toContain('Reached in 5 steps')
  })

  it('shows how steady each router was when the rounds were measured, and the round it is on', () => {
    const t = trace({
      state: 'running', repeat: true, rounds: 10, rounds_done: 3,
      hops: [{ ttl: 1, address: '192.168.50.1', sent: 3, answered: 3, best_ms: 1.6, where: 'this_network', stats: { sent: 4, answered: 4, best_ms: 1.1, avg_ms: 1.5, worst_ms: 2.2 } }],
    })
    const w = mount(TrafficRouteSteps, { props: { trace: t } })
    expect(w.get('[data-test="route-hop-stats"]').text()).toContain('Answered 4/4')
    expect(w.get('[data-test="route-hop-stats"]').text()).toContain('Worst 2.2 ms')
    expect(w.get('[data-test="route-rounds"]').text()).toBe('Measuring, round 4 of 10')
    expect(w.find('[data-test="route-fold-toggle"]').exists()).toBe(false)
  })
})
