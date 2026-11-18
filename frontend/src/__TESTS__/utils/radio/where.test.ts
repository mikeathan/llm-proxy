import { describe, expect, it } from 'vitest'
import { whereOf } from '../../../utils/radio/where'
import type { RadioLiveDevice, RadioRouterSeen } from '../../../types/radio'

const router = (dbm: number, over: Partial<RadioRouterSeen> = {}): RadioRouterSeen => ({ box: 'b1', box_name: 'ZenWiFi_XD4-28F0', band: '5G', dbm, at: 1, ...over })
const dev = (over: Partial<RadioLiveDevice>): Pick<RadioLiveDevice, 'ring' | 'heard' | 'router'> => ({ ring: 'house', heard: true, ...over })

describe('whereOf', () => {
  it('says near this computer when the radar puts it in the sensor’s room, whatever box it is joined to', () => {
    expect(whereOf(dev({ ring: 'room', router: router(-80) }))).toBe('Near this computer')
  })

  it('says near a box when that box hears it strongly', () => {
    expect(whereOf(dev({ ring: 'house', router: router(-55) }))).toBe('Near ZenWiFi_XD4-28F0')
    expect(whereOf(dev({ ring: 'nearby', router: router(-60, { box_name: undefined, main: true }) }))).toBe('Near the main router')
  })

  it('says only which box it is joined to when no listener hears it strongly, never a place between them', () => {
    expect(whereOf(dev({ ring: 'house', router: router(-72) }))).toBe('In the house, joined to ZenWiFi_XD4-28F0')
    expect(whereOf(dev({ ring: 'nearby', router: router(-72) }))).toBe('Far from this computer, joined to ZenWiFi_XD4-28F0')
    expect(whereOf(dev({ heard: false, router: router(-72) }))).toBe('Joined to ZenWiFi_XD4-28F0')
  })

  it('falls back to the radar alone without the router, and says nothing for a device neither hears', () => {
    expect(whereOf(dev({ ring: 'house' }))).toBe('In the house')
    expect(whereOf(dev({ ring: 'nearby' }))).toBe('Far from this computer')
    expect(whereOf(dev({ ring: 'far' }))).toBe('')
    expect(whereOf(dev({ heard: false }))).toBe('')
  })

  it('names a guest network', () => {
    expect(whereOf(dev({ ring: 'house', router: router(-72, { guest: true }) }))).toBe('In the house, joined to ZenWiFi_XD4-28F0 (guest)')
  })
})
