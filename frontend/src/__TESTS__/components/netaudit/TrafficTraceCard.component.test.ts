import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RouteTrace, RouteTracesState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ routeTraces: vi.fn(), startRouteTrace: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

const trace = (over: Partial<RouteTrace> = {}): RouteTrace => ({ address: '9.9.9.9', state: 'running', started_at: '2026-11-11T14:00:00Z', reached: false, hops: [], ...over })
const state = (traces: RouteTrace[], supported = true): RouteTracesState => ({ supported, traces })

describe('TrafficTraceCard', () => {
  beforeEach(() => {
    vi.resetModules()
    service.routeTraces.mockReset()
    service.startRouteTrace.mockReset()
    service.routeTraces.mockResolvedValue(state([]))
  })
  const show = async () => {
    const { default: Card } = await import('../../../components/netaudit/TrafficTraceCard.vue')
    const { useRouteTrace } = await import('../../../composables/netaudit/useRouteTrace')
    await useRouteTrace().load()
    return mount(Card)
  }

  it('cannot trace an empty address, and a preset fills the box in', async () => {
    const w = await show()
    expect(w.get('[data-test="trace-go"]').attributes('disabled')).toBeDefined()
    await w.findAll('[data-test="trace-preset"]')[2]!.trigger('click')
    expect((w.get('[data-test="trace-address"]').element as HTMLInputElement).value).toBe('9.9.9.9')
    expect(w.get('[data-test="trace-go"]').attributes('disabled')).toBeUndefined()
  })

  it('traces the typed address, trimmed, and shows its steps and its route on the map', async () => {
    service.startRouteTrace.mockResolvedValue(state([trace({
      hops: [
        { ttl: 1, address: '192.168.50.1', sent: 3, answered: 3, best_ms: 1.6, where: 'this_network' },
        { ttl: 2, address: '90.1.1.1', sent: 3, answered: 3, best_ms: 2.4, where: 'internet', owner: { asn: 5607, name: 'BSKYB', country: 'GB', place_country: 'GB', city: 'London', point: { lat: 51.5, lon: -0.1 } } },
        { ttl: 3, address: '64.125.1.1', sent: 3, answered: 3, best_ms: 73, where: 'internet', owner: { asn: 6461, name: 'Zayo', country: 'US', place_country: 'US', city: 'Newark', point: { lat: 40.7, lon: -74.2 } } },
      ],
    })]))
    const w = await show()
    await w.get('[data-test="trace-address"]').setValue('  9.9.9.9 ')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.startRouteTrace).toHaveBeenCalledWith('9.9.9.9', false)
    expect(w.get('[data-test="trace-result-address"]').text()).toBe('9.9.9.9')
    expect(w.findAll('[data-test="route-hop"]')).toHaveLength(3)
    expect(w.findAll('[data-test="map-route-point"]')).toHaveLength(2)
    expect(w.find('[data-test="map-dot"]').exists()).toBe(false)
  })

  it('asks for the rounds when the measuring button is pressed', async () => {
    service.startRouteTrace.mockResolvedValue(state([trace()]))
    const w = await show()
    await w.get('[data-test="trace-address"]').setValue('1.1.1.1')
    await w.get('[data-test="trace-repeat"]').trigger('click')
    await flushPromises()
    expect(service.startRouteTrace).toHaveBeenCalledWith('1.1.1.1', true)
  })

  it('says why a trace was refused, in the server\'s words, and keeps the box', async () => {
    service.startRouteTrace.mockRejectedValue(new Error('that address is on the blocked list in Settings → Guardrails'))
    const w = await show()
    await w.get('[data-test="trace-address"]').setValue('1.1.1.1')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.get('[data-test="trace-error"]').text()).toContain('blocked list')
    expect(w.find('[data-test="trace-result"]').exists()).toBe(false)
  })

  it('keeps the result of its own trace across a visit to another tab, and shows no other kept trace', async () => {
    service.routeTraces.mockResolvedValue(state([trace({ address: '8.8.8.8', state: 'done', reached: true })]))
    expect((await show()).find('[data-test="trace-result"]').exists()).toBe(false)
    service.startRouteTrace.mockResolvedValue(state([trace({ address: '1.1.1.1' })]))
    const w = await show()
    await w.get('[data-test="trace-address"]').setValue('1.1.1.1')
    await w.get('form').trigger('submit')
    await flushPromises()
    w.unmount()
    const { default: Card } = await import('../../../components/netaudit/TrafficTraceCard.vue')
    expect(mount(Card).get('[data-test="trace-result-address"]').text()).toBe('1.1.1.1')
  })

  it('says a trace needs Linux elsewhere instead of offering the box', async () => {
    service.routeTraces.mockResolvedValue(state([], false))
    const w = await show()
    expect(w.get('[data-test="trace-unsupported"]').text()).toContain('needs Linux')
    expect(w.find('[data-test="trace-address"]').exists()).toBe(false)
  })
})
