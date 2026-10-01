// User-visible numbers (plan D24): components never format inline.

const COMPACT_UNITS: [number, string][] = [
  [1e9, 'B'],
  [1e6, 'M'],
  [1e3, 'K'],
]
const SMALL_COST_THRESHOLD = 0.01
const SMALL_COST_DECIMALS = 5
const COST_DECIMALS = 4

/** Compact token count: `334.1M`, `1.5K`, `999`. */
export function formatTokenCount(n: number): string {
  const unit = COMPACT_UNITS.find(([size]) => n >= size)
  return unit ? `${(n / unit[0]).toFixed(1)}${unit[1]}` : String(n)
}

/** USD cost; sub-cent amounts keep five decimals so they never read as $0. */
export function formatCost(usd: number): string {
  return `$${usd.toFixed(usd < SMALL_COST_THRESHOLD ? SMALL_COST_DECIMALS : COST_DECIMALS)}`
}

const RATIO_DECIMALS = 2

/** A WCAG contrast ratio, e.g. `4.50:1` (theme editor). */
export function formatContrastRatio(ratio: number): string {
  return `${ratio.toFixed(RATIO_DECIMALS)}:1`
}

const MB_PER_GB = 1024
const FIGURE_DECIMALS = 1

/** A percentage with one decimal, e.g. `14.3%` (host stats). */
export function formatPercent(percent: number): string {
  return `${percent.toFixed(FIGURE_DECIMALS)}%`
}

/** Tokens per second with one decimal, e.g. `33.2` (the unit is shown beside it). */
export function formatTokenRate(tokensPerSecond: number): string {
  return tokensPerSecond.toFixed(FIGURE_DECIMALS)
}

/** Memory in use out of the total, e.g. `7.3 / 46.8 GB`. */
export function formatMemoryUsage(usedMb: number, totalMb: number): string {
  return `${(usedMb / MB_PER_GB).toFixed(FIGURE_DECIMALS)} / ${(totalMb / MB_PER_GB).toFixed(FIGURE_DECIMALS)} GB`
}
