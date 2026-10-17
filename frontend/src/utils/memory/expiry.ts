// The last calendar day a fact is still valid, as YYYY-MM-DD ('' when it never expires). The backend stores an
// expiry as an instant: a dated fact expires at the start of the day after its date, a session fact mid-day. Stepping
// back one second makes both read as the day the operator would name (mirror of backend MemoryEntry.LastDay).
export function expiryLastDay(expiresAt?: string | null): string {
  if (!expiresAt) return ''
  const ms = Date.parse(expiresAt)
  if (Number.isNaN(ms)) return ''
  return new Date(ms - 1000).toISOString().slice(0, 10)
}
