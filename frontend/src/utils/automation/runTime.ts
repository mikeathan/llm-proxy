const TIMESTAMP_PATTERN = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/
const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MONTH_NAMES = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

/**
 * "Mon 12 Oct, 07:00" for an RFC 3339 timestamp, read on the wall clock the
 * timestamp itself carries. A schedule runs on the server's clock, so the
 * browser's zone must not shift it.
 */
export function formatRunTime(iso: string): string {
  const match = TIMESTAMP_PATTERN.exec(iso)
  if (!match) return iso
  const [, year, month, day, hour, minute] = match
  const weekday = DAY_NAMES[new Date(Date.UTC(Number(year), Number(month) - 1, Number(day))).getUTCDay()]
  return `${weekday} ${Number(day)} ${MONTH_NAMES[Number(month) - 1]}, ${hour}:${minute}`
}
