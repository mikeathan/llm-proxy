/**
 * Formats a timestamp string into a locale-aware time string (HH:MM).
 */
export const formatTime = (ts: string): string => {
  return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
};

/**
 * Formats a timestamp string into a locale-aware date string.
 * Returns 'Today' if the date is the current day.
 */
export const formatDate = (ts: string): string => {
  const d = new Date(ts);
  const now = new Date();
  if (d.toDateString() === now.toDateString()) return "Today";
  return d.toLocaleDateString([], { month: "short", day: "numeric" });
};

const MS_PER_SECOND = 1000;
const SECONDS_PER_MINUTE = 60;
const SECONDS_PER_HOUR = 3600;
const RELATIVE_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["day", 86400],
  ["hour", SECONDS_PER_HOUR],
  ["minute", SECONDS_PER_MINUTE],
  ["second", 1],
];

/**
 * A run duration (plan D24): `8.403s` under a minute, `2m 5s` under an hour,
 * `3h 7m` beyond.
 */
export const formatDuration = (ms: number): string => {
  const seconds = ms / MS_PER_SECOND;
  if (seconds < SECONDS_PER_MINUTE) return `${seconds.toFixed(3)}s`;
  const h = Math.floor(seconds / SECONDS_PER_HOUR);
  const m = Math.floor((seconds % SECONDS_PER_HOUR) / SECONDS_PER_MINUTE);
  return h > 0 ? `${h}h ${m}m` : `${m}m ${Math.round(seconds % SECONDS_PER_MINUTE)}s`;
};

/** Whole seconds as people say them: `42s`, `1m 5s`, `1h 2m`. */
export const formatElapsedSeconds = (total: number): string => {
  const seconds = Math.max(0, Math.floor(total));
  if (seconds < SECONDS_PER_MINUTE) return `${seconds}s`;
  const h = Math.floor(seconds / SECONDS_PER_HOUR);
  const m = Math.floor((seconds % SECONDS_PER_HOUR) / SECONDS_PER_MINUTE);
  return h > 0 ? `${h}h ${m}m` : `${m}m ${seconds % SECONDS_PER_MINUTE}s`;
};

/**
 * Relative time in the user's locale (`5 minutes ago`) — for lists, with the
 * absolute value in a tooltip (formatAbsoluteTime).
 */
export const formatRelativeTime = (iso: string, nowMs: number = Date.now(), locale?: string): string => {
  const diff = Math.round((Date.parse(iso) - nowMs) / MS_PER_SECOND);
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const [unit, size] = RELATIVE_UNITS.find(([, s]) => Math.abs(diff) >= s) ?? RELATIVE_UNITS[RELATIVE_UNITS.length - 1]!;
  return rtf.format(unit === "second" && Math.abs(diff) < 10 ? 0 : Math.round(diff / size), unit);
};

/** Absolute date and time in the user's locale and time zone. */
export const formatAbsoluteTime = (iso: string, locale?: string, timeZone?: string): string =>
  new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short", timeZone }).format(new Date(iso));

/**
 * Formats elapsed time since an ISO timestamp as a compact string
 * (e.g. "45s", "3m 12s"). Pass the current time (nowMs) so callers can
 * keep the display reactive with a ticking clock.
 */
export const formatElapsedSince = (iso: string, nowMs: number = Date.now()): string => {
  const secs = Math.max(0, Math.floor((nowMs - new Date(iso).getTime()) / 1000));
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  return `${mins}m ${secs % 60}s`;
};

/**
 * Some stored timestamps carry no zone but are UTC (the memory store); reading
 * them as local time would shift them by the user's offset.
 */
export const asUtc = (ts: string): string => (/[zZ]|[+-]\d\d:?\d\d$/.test(ts) ? ts : `${ts}Z`);
