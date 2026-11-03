import { describe, expect, it } from 'vitest'
import { filterNearby, groupNearby, nearbyKind, nearbyName, nearbyReport, nearbyStatus, nearbySummary, serviceLabel } from '../../utils/netaudit/nearby'
import type { NearbyDevice, NearbyState } from '../../types/netaudit'

const LOOK = '2026-11-03T12:00:00Z'
const dev = (over: Partial<NearbyDevice>): NearbyDevice => ({ ip: '192.168.50.20', first_heard: LOOK, last_heard: LOOK, known: true, new: false, ...over })
const state = (devices: NearbyDevice[], over: Partial<NearbyState> = {}): NearbyState => ({ enabled: true, running: false, last_look: LOOK, last_outcome: 'heard', devices, ...over })

describe('nearby helpers', () => {
  it('reads what a device advertises as from its services, a gateway claim first', () => {
    expect(nearbyKind(dev({ services: ['_ipp._tcp'] }))).toBe('printer')
    expect(nearbyKind(dev({ services: ['_rtsp._tcp', '_http._tcp'] }))).toBe('camera')
    expect(nearbyKind(dev({ upnp_type: 'MediaRenderer:1' }))).toBe('tv')
    expect(nearbyKind(dev({ services: ['_ssh._tcp'] }))).toBe('computer')
    expect(nearbyKind(dev({ services: ['_ipp._tcp'], gateway: true }))).toBe('gateway')
    expect(nearbyKind(dev({}))).toBe('other')
  })

  it('names a device by the person\'s name, then its own, then maker and model, then its address', () => {
    expect(nearbyName(dev({ name: 'Kitchen', names: ['x'] }))).toBe('Kitchen')
    expect(nearbyName(dev({ names: ['Office Printer'] }))).toBe('Office Printer')
    expect(nearbyName(dev({ maker: 'Example', model: 'EN 300' }))).toBe('Example EN 300')
    expect(nearbyName(dev({}))).toBe('192.168.50.20')
  })

  it('strips the characters that reverse the direction of text from a name', () => {
    expect(nearbyName(dev({ names: ['abc\u202egpj.exe\u200b'] }))).toBe('abcgpj.exe')
  })

  it('writes a service type in words and a strange one as its bare name', () => {
    expect(serviceLabel('_ipp._tcp')).toBe('Printing (IPP)')
    expect(serviceLabel('_foo-bar._udp')).toBe('foo-bar')
  })

  it('puts a problem first, then new, and tells a device not in the last look from one that was', () => {
    const later = '2026-11-03T11:00:00Z'
    expect(nearbyStatus(dev({ problem: { check: 'rogue_gateway', title: 't', text: 'x' }, new: true }), LOOK)).toBe('attention')
    expect(nearbyStatus(dev({ new: true }), LOOK)).toBe('new')
    expect(nearbyStatus(dev({}), LOOK)).toBe('heard')
    expect(nearbyStatus(dev({ last_heard: later }), LOOK)).toBe('quiet')
    expect(nearbyStatus(dev({}), undefined)).toBe('quiet')
  })

  it('filters and groups by status', () => {
    const s = state([dev({ ip: '1' }), dev({ ip: '2', new: true }), dev({ ip: '3', last_heard: '2026-11-01T00:00:00Z' })])
    expect(filterNearby(s, 'new').map((d) => d.ip)).toEqual(['2'])
    expect(filterNearby(s, 'all')).toHaveLength(3)
    expect(groupNearby(s.devices, LOOK).map((g) => g.status)).toEqual(['new', 'heard', 'quiet'])
  })

  it('says silence is not proof, never that nothing is there', () => {
    expect(nearbySummary(state([], { last_outcome: 'nothing_heard' }))).toContain('does not mean nothing is there')
    expect(nearbySummary(state([], { last_outcome: 'lan_off' }))).toContain('Guardrails')
    expect(nearbySummary(state([], { last_look: undefined, last_outcome: undefined }))).toContain('has not looked yet')
    expect(nearbySummary(state([], { running: true }))).toContain('Looking')
  })

  it('counts the devices and those that need a look', () => {
    const s = state([dev({ problem: { check: 'rogue_gateway', title: 't', text: 'x' } }), dev({ ip: '2' })])
    expect(nearbySummary(s)).toMatch(/^2 devices heard, 1 needs a look\. Last checked /)
  })

  it('keeps names out of the report to copy', () => {
    const json = JSON.stringify(nearbyReport(state([dev({ name: 'Secret Name', names: ['Another'], maker: 'M' })])))
    expect(json).not.toContain('Secret Name')
    expect(json).not.toContain('Another')
  })
})
