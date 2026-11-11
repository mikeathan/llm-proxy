import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { DestinationDataState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ destinationData: vi.fn(), downloadDestinationData: vi.fn(), removeDestinationData: vi.fn(), destinationPlaces: vi.fn(), downloadDestinationPlaces: vi.fn(), removeDestinationPlaces: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import DestinationDataCard from '../../../components/netaudit/DestinationDataCard.vue'

const state = (over: Partial<DestinationDataState> = {}): DestinationDataState => ({ ranges: 0, source: 'iptoasn.com', ...over })
const mountCard = async () => {
  const w = mount(DestinationDataCard)
  await flushPromises()
  return w
}

describe('DestinationDataCard', () => {
  beforeEach(() => { for (const f of Object.values(service)) f.mockReset(); service.destinationData.mockResolvedValue(state()) })

  it('says what it does, that lookups stay on this computer, and who a download asks', async () => {
    const w = await mountCard()
    expect(w.text()).toContain('every lookup happens on this computer')
    expect(w.get('[data-test="destination-data-line"]').text()).toContain('Not downloaded yet')
    expect(w.get('[data-test="destination-data-source"]').text()).toContain('iptoasn.com')
    expect(w.get('[data-test="destination-data-source"]').text()).toContain('passphrase')
    expect(w.find('[data-test="destination-data-remove"]').exists()).toBe(false)
  })

  it('downloads the table and then says how many ranges are held', async () => {
    service.downloadDestinationData.mockResolvedValue(state({ ranges: 579453, loaded_at: new Date().toISOString() }))
    const w = await mountCard()
    await w.get('[data-test="destination-data-download"]').trigger('click')
    await flushPromises()
    expect(service.downloadDestinationData).toHaveBeenCalledTimes(1)
    expect(w.get('[data-test="destination-data-line"]').text()).toContain('579,453 address ranges')
    expect(w.get('[data-test="destination-data-download"]').text()).toBe('Refresh the table')
    expect(w.find('[data-test="destination-data-source"]').exists()).toBe(false)
  })

  it('shows why a download failed and keeps what was held', async () => {
    service.destinationData.mockResolvedValue(state({ ranges: 10, loaded_at: new Date().toISOString() }))
    service.downloadDestinationData.mockRejectedValue(new Error('iptoasn.com answered 503'))
    const w = await mountCard()
    await w.get('[data-test="destination-data-download"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="destination-data-error"]').text()).toContain('503')
    expect(w.get('[data-test="destination-data-line"]').text()).toContain('10 address ranges')
  })

  it('removes the table', async () => {
    service.destinationData.mockResolvedValue(state({ ranges: 10, loaded_at: new Date().toISOString() }))
    service.removeDestinationData.mockResolvedValue(state())
    const w = await mountCard()
    await w.get('[data-test="destination-data-remove"]').trigger('click')
    await flushPromises()
    expect(service.removeDestinationData).toHaveBeenCalledTimes(1)
    expect(w.get('[data-test="destination-data-line"]').text()).toContain('Not downloaded yet')
  })

  it('says when the table could not be read at all', async () => {
    service.destinationData.mockRejectedValue(new Error('boom'))
    const w = await mountCard()
    expect(w.find('[data-test="destination-data-error"]').exists()).toBe(true)
    expect(w.find('[data-test="destination-data-download"]').exists()).toBe(false)
  })
})

describe('DestinationDataCard for the table of cities', () => {
  beforeEach(() => { for (const f of Object.values(service)) f.mockReset(); service.destinationPlaces.mockResolvedValue(state({ source: 'db-ip.com' })) })
  const mountPlaces = async () => {
    const w = mount(DestinationDataCard, { props: { table: 'places' } })
    await flushPromises()
    return w
  }

  it('says it is optional, how big it is, that it costs almost no memory, who a download asks and who to credit', async () => {
    const w = await mountPlaces()
    expect(w.text()).toContain('Cities (optional)')
    expect(w.text()).toContain('86 MB')
    expect(w.text()).toContain('almost no memory')
    expect(w.get('[data-test="destination-places-source"]').text()).toContain('db-ip.com')
    expect(w.get('[data-test="destination-places-credit"]').text()).toContain('DB-IP')
    expect(service.destinationData).not.toHaveBeenCalled()
  })

  it('downloads through its own route and says the approximation once a table is held', async () => {
    service.downloadDestinationPlaces.mockResolvedValue(state({ ranges: 7255909, loaded_at: new Date().toISOString(), source: 'db-ip.com' }))
    const w = await mountPlaces()
    await w.get('[data-test="destination-places-download"]').trigger('click')
    await flushPromises()
    expect(service.downloadDestinationPlaces).toHaveBeenCalledTimes(1)
    expect(service.downloadDestinationData).not.toHaveBeenCalled()
    expect(w.get('[data-test="destination-places-line"]').text()).toContain('7,255,909')
    expect(w.text()).toContain('approximate')
  })

  it('removes the table of cities through its own route', async () => {
    service.destinationPlaces.mockResolvedValue(state({ ranges: 10, loaded_at: new Date().toISOString(), source: 'db-ip.com' }))
    service.removeDestinationPlaces.mockResolvedValue(state({ source: 'db-ip.com' }))
    const w = await mountPlaces()
    await w.get('[data-test="destination-places-remove"]').trigger('click')
    await flushPromises()
    expect(service.removeDestinationPlaces).toHaveBeenCalledTimes(1)
    expect(w.get('[data-test="destination-places-line"]').text()).toContain('Not downloaded yet')
  })
})

describe('DestinationDataCard age', () => {
  beforeEach(() => { for (const f of Object.values(service)) f.mockReset() })
  const daysAgo = (n: number) => new Date(Date.now() - n * 86_400_000).toISOString()

  it('says nothing about a fresh table and nudges about one past three months', async () => {
    service.destinationData.mockResolvedValue(state({ ranges: 10, loaded_at: daysAgo(30) }))
    expect((await mountCard()).find('[data-test="destination-data-old"]').exists()).toBe(false)
    service.destinationData.mockResolvedValue(state({ ranges: 10, loaded_at: daysAgo(200) }))
    const w = await mountCard()
    expect(w.get('[data-test="destination-data-old"]').text()).toContain('about 7 months old')
    expect(w.get('[data-test="destination-data-old"]').text()).toContain('Refresh')
  })

  it('says nothing when no table is held', async () => {
    service.destinationData.mockResolvedValue(state())
    expect((await mountCard()).find('[data-test="destination-data-old"]').exists()).toBe(false)
  })
})
