import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import type { WifiState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ wifi: vi.fn(), wifiLook: vi.fn(), addWifiNetwork: vi.fn(), removeWifiNetwork: vi.fn(), confirmWifi: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import WifiCard from '../../../components/netaudit/WifiCard.vue'

const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', name: 'home', component: { template: '<div/>' } }, { path: '/network/:tab?', name: 'network', component: { template: '<div/>' } }] })
const ROUTER = 'aa:bb:cc:00:00:01'
const TWIN = 'de:ad:be:ef:00:09'

const state = (over: Partial<WifiState> = {}): WifiState => ({
  enabled: true, ready: true, last_look: new Date(Date.now() - 60_000).toISOString(),
  networks: [{ name: 'home', confirmed: [ROUTER], seen: [{ bssid: ROUTER, signal: 55, security: 'wpa2', confirmed: true }, { bssid: TWIN, signal: 40, security: 'open', confirmed: false }] }],
  ...over,
})
const mountIt = async () => {
  const w = mount(WifiCard, { global: { plugins: [router] } })
  await flushPromises()
  return w
}

describe('WifiCard', () => {
  beforeEach(() => { Object.values(service).forEach((f) => f.mockReset()); service.wifi.mockResolvedValue(state()) })

  it('lists what advertises the name with its signal and security, and which are confirmed', async () => {
    const w = await mountIt()
    const rows = w.findAll('[data-test="wifi-seen"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain(ROUTER)
    expect(rows[0]!.text()).toContain('Confirmed as yours')
    expect(rows[0]!.find('input[type="checkbox"]').exists()).toBe(false)
    expect(rows[1]!.text()).toContain('40% signal')
    expect(rows[1]!.text()).toContain('no security')
    expect(rows[1]!.text()).toContain('Not confirmed')
  })

  it('says why the look cannot run and does not offer it', async () => {
    service.wifi.mockResolvedValue(state({ ready: false, why: 'nmcli is not installed' }))
    const w = await mountIt()
    expect(w.get('[data-test="wifi-why"]').text()).toContain('nmcli is not installed')
    expect(w.get('[data-test="wifi-look"]').attributes('disabled')).toBeDefined()
  })

  it('says a name nothing is confirmed for is not judged yet', async () => {
    service.wifi.mockResolvedValue(state({ networks: [{ name: 'home', confirmed: [], seen: [] }] }))
    const w = await mountIt()
    expect(w.get('[data-test="wifi-unconfirmed"]').text()).toContain('nothing is reported for this name')
  })

  it('adds a name, then clears the field', async () => {
    service.addWifiNetwork.mockResolvedValue(state())
    const w = await mountIt()
    await w.get('[data-test="wifi-name"]').setValue('  home  ')
    await w.get('[data-test="wifi-add"]').trigger('submit')
    await flushPromises()
    expect(service.addWifiNetwork).toHaveBeenCalledWith('home')
    expect((w.get('[data-test="wifi-name"]').element as HTMLInputElement).value).toBe('')
  })

  it('looks on request and confirms only the ticked access points', async () => {
    service.wifiLook.mockResolvedValue(state())
    service.confirmWifi.mockResolvedValue(state())
    const w = await mountIt()
    await w.get('[data-test="wifi-look"]').trigger('click')
    await flushPromises()
    expect(service.wifiLook).toHaveBeenCalledTimes(1)
    expect(w.get('[data-test="wifi-confirm"]').attributes('disabled')).toBeDefined()
    await w.get(`input[aria-label="${TWIN} is mine"]`).setValue(true)
    await w.get('[data-test="wifi-confirm"]').trigger('click')
    await flushPromises()
    expect(service.confirmWifi).toHaveBeenCalledWith('home', [TWIN])
  })

  it('puts the server’s words beside the control when it refuses', async () => {
    service.confirmWifi.mockRejectedValue(new Error('de:ad:be:ef:00:09 was not seen advertising "home" in the last look; look again first'))
    const w = await mountIt()
    await w.get(`input[aria-label="${TWIN} is mine"]`).setValue(true)
    await w.get('[data-test="wifi-confirm"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="wifi-error"]').text()).toContain('look again first')
  })

  it('stops watching a name', async () => {
    service.removeWifiNetwork.mockResolvedValue(state({ networks: [] }))
    const w = await mountIt()
    await w.get('[data-test="wifi-remove"]').trigger('click')
    await flushPromises()
    expect(service.removeWifiNetwork).toHaveBeenCalledWith('home')
    expect(w.find('[data-test="wifi-network-home"]').exists()).toBe(false)
  })

  it('shows the last failed look as a problem', async () => {
    service.wifi.mockResolvedValue(state({ problem: 'NetworkManager is not running' }))
    const w = await mountIt()
    expect(w.get('[data-test="wifi-problem"]').text()).toContain('NetworkManager is not running')
  })
})
