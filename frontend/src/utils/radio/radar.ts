// Radio · the radar's geometry: how far its edge reaches (the zoom), where a distance falls on it, and which rings to draw. Pure functions.
// The scale is logarithmic from one metre at the centre to the edge, so the room and the street both fit; zooming moves the edge.

/** The edges the zoom steps through, in metres; the last shows everything the model can place. */
export const RADAR_EDGES = [5, 10, 20, 40, 80, 200] as const
export const RADAR_DEFAULT_EDGE = 80
const RING_CHOICES = [1, 2, 3, 5, 10, 20, 30, 50, 100, 200]

/** The next edge in (smaller) or out (bigger); it stays at the ends. */
export function zoomStep(edge: number, dir: 'in' | 'out'): number {
  const i = RADAR_EDGES.findIndex((e) => e >= edge)
  const at = i < 0 ? RADAR_EDGES.length - 1 : i
  const next = dir === 'in' ? Math.max(0, at - 1) : Math.min(RADAR_EDGES.length - 1, at + (RADAR_EDGES[at] === edge ? 1 : 0))
  return RADAR_EDGES[next] ?? RADAR_DEFAULT_EDGE
}

/** Where `metres` falls between the centre (0) and the edge (maxR), and whether it is past the edge (drawn on it, faded). */
export function radiusOf(metres: number, edge: number, maxR: number): { r: number; beyond: boolean } {
  const m = Math.max(1, metres)
  const beyond = m > edge
  return { r: (maxR * Math.log(Math.min(m, edge))) / Math.log(Math.max(2, edge)), beyond }
}

/** Up to four rings inside the edge, spread over the scale. */
export function ringsFor(edge: number): number[] {
  const inside = RING_CHOICES.filter((m) => m > 1 && m < edge)
  if (inside.length <= 4) return [...inside, edge]
  const step = inside.length / 3
  return [0, 1, 2].map((i) => inside[Math.floor(i * step)] as number).concat(edge)
}

// The auto edge is framed on the typical device, not the farthest (borrowed from nodeherder's radar): one far neighbour's meter would otherwise
// push the scale out and crush the house into the middle. A set with no far tail is covered completely; past the tail, the far ones sit on the
// rim and are counted.
const FIT_MEDIAN_MULTIPLE = 1.5
const FIT_TAIL_RATIO = 2

/** The edge the radar opens at for these middle distances. */
export function edgeToFit(middles: number[]): number {
  const sorted = middles.filter((m) => Number.isFinite(m) && m > 0).sort((a, b) => a - b)
  const last = RADAR_EDGES[RADAR_EDGES.length - 1] ?? RADAR_DEFAULT_EDGE
  if (!sorted.length) return RADAR_EDGES[0] ?? RADAR_DEFAULT_EDGE
  const far = sorted[sorted.length - 1] ?? 1
  const mid = sorted.length % 2 ? (sorted[(sorted.length - 1) / 2] ?? far) : ((sorted[sorted.length / 2 - 1] ?? far) + (sorted[sorted.length / 2] ?? far)) / 2
  // One device has no spread, so headroom would only shrink it; a set with no far tail is covered completely.
  const want = sorted.length === 1 ? far : far <= mid * FIT_TAIL_RATIO ? Math.max(far, mid * FIT_MEDIAN_MULTIPLE) : mid * FIT_MEDIAN_MULTIPLE
  return RADAR_EDGES.find((e) => e >= want) ?? last
}

/** Label boxes that do not overlap one already placed, in order (the first ones win): a crowded radar keeps its labels readable. */
export function placeLabels<T>(items: T[], box: (t: T) => { x: number; y: number; w: number; h: number }): Set<T> {
  const kept: { x: number; y: number; w: number; h: number }[] = []
  const out = new Set<T>()
  for (const it of items) {
    const b = box(it)
    if (kept.some((k) => b.x < k.x + k.w && k.x < b.x + b.w && b.y < k.y + k.h && k.y < b.y + b.h)) continue
    kept.push(b)
    out.add(it)
  }
  return out
}
