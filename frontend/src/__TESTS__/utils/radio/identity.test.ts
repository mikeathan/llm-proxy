import { describe, expect, it } from 'vitest'
import { brandModel, conclusionOf, defaultLabel, evidenceOf, nameOf, ownershipOf, shortBrand } from '../../../utils/radio/identity'
import type { RadioIdent, RadioLiveDevice } from '../../../types/radio'

const ident = (over: Partial<RadioIdent>): RadioIdent => ({ kind: 'unknown', kind_confidence: 0, kind_source: 'no_evidence', confidence: 0, ...over })
const dev = (over: Partial<RadioLiveDevice>): RadioLiveDevice => ({
  id: '2a6e3b0d', mine: false, class: 'transient', dbm: -70, near_m: 6, far_m: 20, ring: 'nearby', angle: 10, trend: 'steady', channel: 6, heard: true, ...over,
})

describe('radio identity', () => {
  it('names a neighbour’s Sky router by what it is, never by its short id', () => {
    const sky = dev({ ident: ident({ kind: 'router', brand: 'SKY UK', kind_source: 'sends_beacons', confidence: 70 }), class: 'neighbour' })
    expect(nameOf(sky)).toBe('SKY UK router')
    expect(nameOf(sky)).not.toContain(sky.id)
    expect(conclusionOf(sky)).toBe('Probably: a neighbour’s router')
    expect(ownershipOf(sky)).toBe('neighbour')
  })

  it('says a device is likely yours from the evidence, and only your word makes it yours', () => {
    expect(ownershipOf(dev({ ident: ident({ on_your_wifi: true }) }))).toBe('likely')
    expect(ownershipOf(dev({ ident: ident({ lan_name: 'Kitchen iPad' }) }))).toBe('likely')
    expect(ownershipOf(dev({ ident: ident({ your_ap: true }) }))).toBe('likely')
    expect(ownershipOf(dev({ mine: true, label: 'x' }))).toBe('mine')
    expect(conclusionOf(dev({ ident: ident({ lan_name: 'Kitchen iPad' }) }))).toBe('On your network as “Kitchen iPad”')
    expect(conclusionOf(dev({ ident: ident({ your_ap: true }) }))).toBe('Your router')
  })

  it('joins brand and model without repeating the brand, and trims company words', () => {
    expect(brandModel(ident({ brand: 'ASUSTek COMPUTER INC.', model: 'RT-AX88U' }))).toBe('ASUSTek RT-AX88U')
    expect(brandModel(ident({ brand: 'Netgear', model: 'Netgear Orbi RBR50' }))).toBe('Netgear Orbi RBR50')
    expect(brandModel(ident({ brand: 'Apple', brand_hint: true, model: '' }))).toBe('')
    expect(shortBrand('Hui Zhou Gaoshengda Technology Co.,LTD')).toBe('Hui Zhou')
  })

  it('lists the evidence for whose it is, or says there is none', () => {
    const words = { yourAP: 'ap', boxYours: 'box', router: 'router', lan: 'lan', netYours: 'net-yours', netOther: 'net-other', none: 'none', onWiFi: (n: number) => `wifi ${n}` }
    expect(evidenceOf(dev({ ident: ident({ on_your_wifi: true, own_cycles: 4, lan_name: 'x' }) }), words)).toEqual(['lan', 'wifi 4'])
    expect(evidenceOf(dev({ ident: ident({ net: 'ab', net_yours: false }) }), words)).toEqual(['net-other'])
    expect(evidenceOf(dev({}), words)).toEqual(['none'])
  })

  it('offers a label from make and kind, never the name on your network or the network it broadcasts', () => {
    expect(defaultLabel(dev({ ident: ident({ lan_name: 'Kitchen iPad', brand: 'Apple', kind: 'phone' }), network: 'Smiths' }))).toBe('Apple phone or tablet')
    expect(defaultLabel(dev({ network: 'Smiths' }))).toBe('My device')
    expect(defaultLabel(dev({ ident: ident({ brand: 'Sky UK Ltd', model: 'SR203' }) }))).toBe('Sky UK SR203')
  })
})
