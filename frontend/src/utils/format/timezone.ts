// IANA timezone helpers for the reminder settings.

/** The browser's own IANA timezone. */
export const browserTimeZone = (): string => Intl.DateTimeFormat().resolvedOptions().timeZone

/** True for an empty value (unset) or a name the runtime knows. */
export const isKnownTimeZone = (name: string): boolean => {
  if (!name) return true
  // A bare abbreviation like "BST" or an offset is not an IANA name.
  if (!name.includes('/') && name !== 'UTC') return false
  try {
    new Intl.DateTimeFormat(undefined, { timeZone: name })
    return true
  } catch {
    return false
  }
}

/** Every IANA zone the runtime knows, for autocomplete. */
export const knownTimeZones = (): string[] =>
  (Intl as unknown as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf?.('timeZone') ?? []

const UTC = 'UTC'
const INPUT_PATTERN = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/
const INPUT_LENGTH = 'YYYY-MM-DDTHH:mm'.length
const MS_PER_MINUTE = 60_000

/** The zone to format in: the given one if the runtime knows it, else UTC. */
const usableZone = (zone: string): string => (zone && isKnownTimeZone(zone) && zone !== 'Local' ? zone : UTC)

/** Wall-clock parts of an instant in a zone. */
function partsInZone(date: Date, zone: string): Record<string, string> {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: zone, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }).formatToParts(date)
  return Object.fromEntries(parts.map((p) => [p.type, p.value]))
}

/** A UTC instant as the `YYYY-MM-DDTHH:mm` an `<input type="datetime-local">` takes, in the given zone. */
export function toZonedInput(iso: string, zone: string): string {
  const date = new Date(iso)
  if (!iso || Number.isNaN(date.getTime())) return ''
  const p = partsInZone(date, usableZone(zone))
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}`
}

/**
 * The UTC instant (ISO string) for wall-clock time typed in a zone, or null when
 * it is not a date. The zone's offset at that moment is found from the zone
 * itself, so a clock change is respected.
 */
export function fromZonedInput(local: string, zone: string): string | null {
  const asUtc = INPUT_PATTERN.test(local) ? Date.parse(`${local}:00Z`) : NaN
  if (Number.isNaN(asUtc)) return null
  const tz = usableZone(zone)
  // Guess the instant as if it were UTC, then correct by how far the zone's wall clock is from it.
  let guess = asUtc
  for (let i = 0; i < 2; i++) {
    const shown = Date.parse(`${toZonedInput(new Date(guess).toISOString(), tz).slice(0, INPUT_LENGTH)}:00Z`)
    guess += asUtc - shown
  }
  return new Date(Math.round(guess / MS_PER_MINUTE) * MS_PER_MINUTE).toISOString()
}
