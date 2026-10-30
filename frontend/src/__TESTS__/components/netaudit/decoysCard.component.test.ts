import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { DecoyState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ decoys: vi.fn(), setDecoys: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import DecoysCard from '../../../components/netaudit/DecoysCard.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const PORTS = [{ number: 2222, name: 'SSH login' }, { number: 8023, name: 'Telnet' }]
const state = (over: Partial<DecoyState> = {}): DecoyState => ({ enabled: false, listening: [], ports: PORTS, touches: [], ...over })

async function mountCard() {
  const w = mount(DecoysCard)
  await flushPromises()
  return w
}

describe('DecoysCard', () => {
  beforeEach(() => {
    service.decoys.mockReset().mockResolvedValue(state())
    service.setDecoys.mockReset()
    useNetauditViewMode().set('simple')
  })

  it('is off until the person turns it on, and says nothing has connected', async () => {
    const w = await mountCard()
    expect((w.get('[role="switch"]').element as HTMLInputElement).checked).toBe(false)
    expect(w.text()).toContain('Nothing has connected to a decoy')
  })

  it('turns the decoys on and shows the firewall note', async () => {
    service.setDecoys.mockResolvedValue(state({ enabled: true, listening: [2222, 8023] }))
    const w = await mountCard()
    await w.get('[role="switch"]').setValue(true)
    await flushPromises()
    expect(service.setDecoys).toHaveBeenCalledWith(true)
    expect(w.text()).toContain('firewall')
  })

  it('lists who connected, never what they sent', async () => {
    service.decoys.mockResolvedValue(state({ enabled: true, touches: [{ source: '192.168.50.77', port: 2222, first: '2026-10-30T01:00:00Z', last: '2026-10-30T02:00:00Z', count: 3 }] }))
    const w = await mountCard()
    const row = w.get('[data-test="decoy-touches"]').text()
    expect(row).toContain('192.168.50.77')
    expect(row).toContain('3 times')
  })

  it('states a decoy that cannot start', async () => {
    service.decoys.mockResolvedValue(state({ enabled: true, problem: 'port 8023 is in use or not allowed' }))
    const w = await mountCard()
    expect(w.get('[data-test="decoy-problem"]').text()).toContain('8023')
  })

  it('shows the ports only in Advanced', async () => {
    service.decoys.mockResolvedValue(state({ enabled: true, listening: [2222] }))
    const w = await mountCard()
    expect(w.find('[aria-label="Decoy ports"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    await flushPromises()
    expect(w.get('[aria-label="Decoy ports"]').text()).toContain('2222 · listening')
  })

  it('says what failed when the state cannot be read', async () => {
    service.decoys.mockRejectedValue(new Error('down'))
    const w = await mountCard()
    expect(w.text()).toContain('could not be read')
  })
})
