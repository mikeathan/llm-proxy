import { describe, expect, it } from 'vitest'
import { deviceFacts, deviceLabel, deviceMakerNote, deviceTags, deviceTitle, heardAgoWord, trailPoints } from '../../../utils/radio/describe'
import type { RadioAirDevice, RadioLiveDevice } from '../../../types/radio'

const dev = (over: Partial<RadioLiveDevice>): RadioLiveDevice => ({
  id: 'ab12cd34', mine: false, class: 'infrastructure', dbm: -58, near_m: 4, far_m: 12, ring: 'house', angle: 90, trend: 'steady', channel: 36, heard: true, ...over,
})
const detail = (over: Partial<RadioAirDevice>): RadioAirDevice => ({
  id: 'ab12cd34', class: 'infrastructure', why: 'always_here_steady_signal', signal: 'strong', first_seen: '2026-11-06', last_seen: '2026-11-15', cycles_heard: 118, need_heard: 12, retry_percent: 0, channels: [36], random_address: false, joined_own: false, ...over,
})

describe('device description', () => {
  it('titles a device by the owner’s name, else the network it broadcasts, else its maker, else why there is no maker', () => {
    expect(deviceTitle(dev({ mine: true, label: 'my phone' }))).toBe('my phone')
    expect(deviceTitle(dev({ network: 'netsink', detail: detail({ maker: 'TP-Link' }) }))).toBe('netsink')
    expect(deviceTitle(dev({ detail: detail({ maker: 'Espressif' }) }))).toBe('Espressif')
    expect(deviceTitle(dev({ detail: detail({ random_address: true }) }))).toBe('Random address')
    expect(deviceTitle(dev({ detail: detail({}) }))).toBe('Maker not listed')
  })

  it('labels a dot with a name, never with an address, and falls back to the short id', () => {
    expect(deviceLabel(dev({}))).toBe('ab12cd34')
    expect(deviceLabel(dev({ detail: detail({ maker: 'Espressif' }) }))).toBe('Espressif')
  })

  it('says the kind, the Wi-Fi generation and the channel, and keeps the maker beside a network name', () => {
    expect(deviceFacts(dev({ detail: detail({ wifi_generation: 5 }) }))).toEqual(['Always here', 'Access point', 'Wi-Fi 5', 'channel 36, 5 GHz'])
    expect(deviceFacts(dev({ mine: true, label: 'x' }))).toEqual(['channel 36, 5 GHz'])
    expect(deviceMakerNote(dev({ network: 'netsink', detail: detail({ maker: 'TP-Link' }) }))).toBe('TP-Link')
    expect(deviceMakerNote(dev({ detail: detail({ maker: 'TP-Link' }) }))).toBe('')
  })

  it('says how long ago it was heard, or nothing when this run has not heard it', () => {
    expect(heardAgoWord(dev({ last_at: 1000 }), 1040)).toBe('heard 40 s ago')
    expect(heardAgoWord(dev({ last_at: 1000 }), 1000 + 12 * 60)).toBe('heard 12 min ago')
    expect(heardAgoWord(dev({}), 1040)).toBe('')
  })

  it('tags a device far from its usual and one still waiting to be sorted', () => {
    const tags = deviceTags(dev({ unusual: true, usual_dbm: -80, shift_db: 21, detail: detail({ why: 'needs_more_listening', cycles_heard: 9 }) }))
    expect(tags.map((t) => t.test)).toEqual(['radio-row-unusual', 'radio-row-tag'])
    expect(tags[1]!.text).toBe('sorted in 3 more cycles')
  })

  it('draws a trail only from two points', () => {
    expect(trailPoints(undefined, 56, 16)).toBe('')
    expect(trailPoints([5], 56, 16)).toBe('')
    expect(trailPoints([5, 5, 5], 56, 16).split(' ')).toHaveLength(3)
  })
})
