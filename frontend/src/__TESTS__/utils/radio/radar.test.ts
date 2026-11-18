import { describe, expect, it } from 'vitest'
import { RADAR_EDGES, edgeToFit, placeLabels, radiusOf, ringsFor, zoomStep } from '../../../utils/radio/radar'

describe('radar geometry', () => {
  it('steps the zoom in and out and stops at the ends', () => {
    expect(zoomStep(20, 'in')).toBe(10)
    expect(zoomStep(20, 'out')).toBe(40)
    expect(zoomStep(RADAR_EDGES[0], 'in')).toBe(RADAR_EDGES[0])
    expect(zoomStep(200, 'out')).toBe(200)
    expect(zoomStep(30, 'out')).toBe(40) // between two edges, out goes to the next bigger one
  })

  it('places a distance on a log scale and puts one past the edge on the rim', () => {
    expect(radiusOf(1, 40, 176).r).toBe(0)
    expect(radiusOf(40, 40, 176).r).toBeCloseTo(176)
    const past = radiusOf(90, 40, 176)
    expect(past.beyond).toBe(true)
    expect(past.r).toBeCloseTo(176)
    expect(radiusOf(10, 20, 176).r).toBeGreaterThan(radiusOf(10, 80, 176).r) // zooming in spreads the near devices out
  })

  it('draws a few rings inside the edge and the edge itself, and fits the farthest device', () => {
    const rings = ringsFor(80)
    expect(rings[rings.length - 1]).toBe(80)
    expect(rings.length).toBeLessThanOrEqual(5)
    expect(edgeToFit([2, 7, 15])).toBe(20)
    expect(edgeToFit([])).toBe(5)
  })
})

describe('radar framing and labels', () => {
  it('frames the typical device, not one far neighbour, and covers a set with no far tail', () => {
    expect(edgeToFit([2, 3, 3.5, 60])).toBe(5) // median 3.25: the far one sits on the rim
    expect(edgeToFit([2, 3, 4])).toBe(5) // no tail: everything is covered
    expect(edgeToFit([7])).toBe(10) // one device: no headroom
  })

  it('keeps the first label and drops one that would overlap it', () => {
    const items = [{ x: 0 }, { x: 10 }, { x: 100 }]
    const kept = placeLabels(items, (i) => ({ x: i.x, y: 0, w: 40, h: 10 }))
    expect([...kept].map((i) => i.x)).toEqual([0, 100])
  })
})
