/**
 * Converts bytes to a human-readable string (B, KB, MB, GB, TB).
 * @example formatBytes(1024) → "1.0 KB"
 */
export function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

/**
 * Formats a raw parameter count into a readable string (e.g., millions or billions).
 * @example formatParameters(7000000000) → "7.0B"
 */
export function formatParameters(params: number): string {
  if (!params) return "";
  if (params >= 1e9) return (params / 1e9).toFixed(1) + "B";
  if (params >= 1e6) return (params / 1e6).toFixed(1) + "M";
  return params.toString();
}

/**
 * Formats a timestamp string into a full locale-aware date+time string.
 * @example formatTS("2026-05-24T16:29:39Z") → "5/24/2026, 4:29:39 PM"
 */
export function formatTS(ts: string): string {
  return new Date(ts).toLocaleString()
}
