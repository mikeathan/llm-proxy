import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TrafficDeviceRail from '../../../components/netaudit/TrafficDeviceRail.vue'
import type { TrafficDevice } from '../../../types/netaudit'

const device = (id: string, name: string, count: number, over: Partial<TrafficDevice> = {}): TrafficDevice => ({
  id, kind: 'router', name, address: `192.168.50.${id.length}`, measure: 'connections', flags: [],
  rows: [{ address: '1.1.1.1', count, source: 'router' }], ...over,
})
const devices: TrafficDevice[] = [
  device('host', 'This computer', 0, { kind: 'host', measure: 'bytes', address: 'wlp2s0', rows: [{ address: '1.1.1.1', count: 1, bytes: 2048, source: 'host' }] }),
  device('a', 'Pixel 8', 62),
  device('bb', 'Living room TV', 31, { flags: ['known_bad'] }),
  device('ccc', 'Doorbell', 9),
]
const show = (selectedId: string | null = 'a') => mount(TrafficDeviceRail, { props: { devices, selectedId } })
const names = (w: ReturnType<typeof show>) => w.findAll('[data-test="rail-device"]').map((d) => d.get('span span').text())

describe('TrafficDeviceRail', () => {
  it('lists this computer first, then the devices that need a look, then the rest busiest first', () => {
    const w = show()
    expect(names(w)).toEqual(['This computer', 'Living room TV', 'Pixel 8', 'Doorbell'])
    expect(w.get('[data-test="rail-section-flagged"]').text()).toBe('Needs a look · 1')
    expect(w.get('[data-test="rail-section-others"]').text()).toBe('On your network · 2')
    expect(w.findAll('[data-test="rail-flag"]')).toHaveLength(1)
  })

  it('shows bytes for this computer and connections for a device of the router, and marks the one in use', () => {
    const w = show('host')
    const rows = w.findAll('[data-test="rail-device"]')
    expect(rows[0]!.text()).toContain('2 KB')
    expect(rows[2]!.text()).toContain('62')
    expect(rows[0]!.attributes('aria-pressed')).toBe('true')
    expect(rows[1]!.attributes('aria-pressed')).toBe('false')
  })

  it('finds a device by name or address, says when none matches, and can list only those that need a look', async () => {
    const w = show()
    await w.get('[data-test="rail-search"]').setValue('door')
    expect(names(w)).toEqual(['Doorbell'])
    await w.get('[data-test="rail-search"]').setValue('nothing here')
    expect(w.get('[data-test="rail-none"]').text()).toBe('No device matches.')
    await w.get('[data-test="rail-search"]').setValue('')
    await w.findAll('[role="radio"]').find((b) => b.text().startsWith('Needs a look'))!.trigger('click')
    expect(names(w)).toEqual(['Living room TV'])
  })

  it('emits the id of the device picked, and closes the phone list after a pick', async () => {
    const w = show()
    await w.get('[data-test="picker-toggle"]').trigger('click')
    expect(w.get('[data-test="picker-toggle"]').attributes('aria-expanded')).toBe('true')
    expect(w.get('[data-test="picker-toggle"]').text()).toContain('Pixel 8')
    await w.findAll('[data-test="rail-device"]')[3]!.trigger('click')
    expect(w.emitted('select')).toEqual([['ccc']])
    expect(w.get('[data-test="picker-toggle"]').attributes('aria-expanded')).toBe('false')
  })
})
