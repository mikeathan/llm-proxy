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

/**
 * Calculates duration between two timestamps or returns the pre-calculated duration.
 */
export const formatDuration = (ms: number): string => {
  return `${ms} ms`;
};

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
