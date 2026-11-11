import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import type { RouterConnState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({
  routerConn: vi.fn(), setRouterTarget: vi.fn(), setRouterKey: vi.fn(), deleteRouterKey: vi.fn(), routerFingerprint: vi.fn(), pinRouter: vi.fn(), readRouter: vi.fn(),
  devices: vi.fn(), routeTraces: vi.fn(),
}))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

const NOW = '2026-11-12T10:00:00Z'
const READY: RouterConnState = { address: '192.168.50.1', user: 'admin', key_saved: true, fingerprint: 'SHA256:abc', watching: false }
const WITH_READ: RouterConnState = {
  ...READY,
  last: {
    at: NOW, lines: 245, kept: 12,
    devices: [
      { address: '192.168.50.220', total: 9, places: [{ address: '52.48.210.116', port: 8883, proto: 'tcp', state: 'ESTABLISHED', count: 9, owner: { asn: 16509, name: 'AMAZON-02', country: 'US', place_country: 'IE', city: 'Dublin' } }] },
      { address: '192.168.50.99', total: 3, places: [{ address: '8.8.8.8', port: 53, proto: 'udp', count: 3 }] },
    ],
  },
}

const show = async () => {
  vi.resetModules()
  const { default: C } = await import('../../../components/netaudit/TrafficRouterSetup.vue')
  const w = mount(C, { global: { stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return w
}

describe('TrafficRouterSetup', () => {
  beforeEach(() => {
    Object.values(service).forEach((f) => f.mockReset())
    service.devices.mockResolvedValue({ devices: [{ mac: 'aa', ip: '192.168.50.220', name: 'Doorbell', first_seen: NOW, last_seen: NOW, approved: true, randomized: false, new: false }] })
    service.routeTraces.mockResolvedValue({ supported: true, traces: [] })
  })

  it('walks the setup in order: where the router is, the key, then trusting the router\'s key', async () => {
    service.routerConn.mockResolvedValue({ address: '', user: '', key_saved: false, watching: false })
    const w = await show()
    expect(w.get('[data-test="router-step"]').text()).toContain('Step 1 of 3')
    service.setRouterTarget.mockResolvedValue({ address: '192.168.50.1', user: 'admin', key_saved: false, watching: false })
    await w.get('[data-test="router-address"]').setValue(' 192.168.50.1 ')
    await w.get('[data-test="router-user"]').setValue('admin')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.setRouterTarget).toHaveBeenCalledWith('192.168.50.1', 'admin')
    expect(w.get('[data-test="router-step"]').text()).toContain('Step 2 of 3')

    service.setRouterKey.mockResolvedValue({ address: '192.168.50.1', user: 'admin', key_saved: true, watching: false })
    await w.get('[data-test="router-key"]').setValue('-----BEGIN OPENSSH PRIVATE KEY-----')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.find('[data-test="router-key"]').exists()).toBe(false)
    expect(w.html()).not.toContain('BEGIN OPENSSH')
    expect(w.get('[data-test="router-step"]').text()).toContain('Step 3 of 3')

    service.routerFingerprint.mockResolvedValue({ fingerprint: 'SHA256:abc' })
    await w.get('[data-test="router-check"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="router-fingerprint"]').text()).toContain('SHA256:abc')
    expect(service.pinRouter).not.toHaveBeenCalled()
    service.pinRouter.mockResolvedValue(READY)
    await w.get('[data-test="router-trust"]').trigger('click')
    await flushPromises()
    expect(service.pinRouter).toHaveBeenCalledWith('SHA256:abc')
    expect(w.find('[data-test="router-setup"]').exists()).toBe(false)
    expect(w.find('[data-test="router-read"]').exists()).toBe(true)
  })

  it('lets the address be changed at the key and trust steps, not only once the setup is finished', async () => {
    service.routerConn.mockResolvedValue({ address: '192.168.50.1', user: 'admin', key_saved: true, watching: false })
    const w = await show()
    expect(w.get('[data-test="router-step"]').text()).toContain('Step 3 of 3')
    expect(w.find('[data-test="router-address"]').exists()).toBe(false)
    await w.get('[data-test="router-change-address"]').trigger('click')
    expect((w.get('[data-test="router-address"]').element as HTMLInputElement).value).toBe('192.168.50.1')
    service.setRouterTarget.mockResolvedValue({ address: '192.168.50.1:2222', user: 'admin', key_saved: true, watching: false })
    await w.get('[data-test="router-address"]').setValue('192.168.50.1:2222')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.setRouterTarget).toHaveBeenCalledWith('192.168.50.1:2222', 'admin')
    expect(w.find('[data-test="router-address"]').exists()).toBe(false)
  })

  it('reads the router on the press and says how much it read, leaving the devices to the device list', async () => {
    service.routerConn.mockResolvedValue(READY)
    const w = await show()
    expect(w.find('[data-test="router-never"]').exists()).toBe(true)
    service.readRouter.mockResolvedValue(WITH_READ)
    await w.get('[data-test="router-read"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="router-when"]').text()).toContain('245 lines, 12 connections')
    expect(w.find('[data-test="router-device"]').exists()).toBe(false)
  })

  it('says the list was cut, and what a refusal was in the server\'s words, and never shows a key', async () => {
    service.routerConn.mockResolvedValue({ ...WITH_READ, last: { ...WITH_READ.last!, truncated: true } })
    const w = await show()
    expect(w.get('[data-test="router-truncated"]').text()).toContain('longer than this program reads')
    service.readRouter.mockRejectedValue(new Error('the router’s SSH key is not the one you trusted'))
    await w.get('[data-test="router-read"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="router-error"]').text()).toContain('not the one you trusted')
    expect(w.html()).not.toContain('PRIVATE KEY-----')
  })

  it('says whether a history is being kept, with a link to the switch when it is not', async () => {
    service.routerConn.mockResolvedValue(READY)
    const off = await show()
    expect(off.get('[data-test="router-watch-off"]').text()).toContain('No history yet')
    expect(off.getComponent(RouterLinkStub).props('to')).toEqual({ name: 'network', params: { tab: 'modules' } })
    service.routerConn.mockResolvedValue({ ...READY, watching: true })
    const on = await show()
    expect(on.get('[data-test="router-watch-on"]').text()).toContain('New companies and countries show as New today')
    expect(on.find('[data-test="router-watch-off"]').exists()).toBe(false)
  })
})
