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
