const DURATION = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/

/** A Go duration such as "1h0m0s" or "5m0s" as the short form the picker uses ("1h", "5m"); anything else is returned as it came. */
export function compactInterval(duration: string): string {
  const m = DURATION.exec(duration)
  if (!m || duration === '') return duration
  const [h, min, s] = [m[1], m[2], m[3]].map((v) => Number(v ?? 0))
  const parts = [h ? `${h}h` : '', min ? `${min}m` : '', s ? `${s}s` : ''].join('')
  return parts || duration
}
