/**
 * Converts megabytes to a formatted GB string.
 * @example mbToGb(1536) → "1.5"
 */
export function mbToGb(mb: number): string {
  return (mb / 1024).toFixed(1)
}

/**
 * Returns a percentage value clamped to [0, 100], formatted to 1 decimal place.
 * @example formatPercent(73.567) → "73.6"
 */
export function formatPercent(value: number | undefined | null): string {
  return (value ?? 0).toFixed(1)
}

/**
 * Returns a RAM usage string in the format "used / total GB".
 * @example formatMemory(2048, 16384) → "2.0 / 16.0 GB"
 */
export function formatMemory(usedMb: number, totalMb: number): string {
  return `${mbToGb(usedMb)} / ${mbToGb(totalMb)} GB`
}

/**
 * Returns the memory utilization percentage as a CSS-ready width string.
 * @example memPercent(1024, 8192) → 12.5
 */
export function memPercent(usedMb: number, totalMb: number): number {
  if (!totalMb) return 0
  return (usedMb / totalMb) * 100
}

/**
 * Formats LLM token throughput to 1 decimal place, returning "0.0" as fallback.
 * @example formatTokenRate(42.1234) → "42.1"
 */
export function formatTokenRate(tps: number | undefined | null): string {
  return (tps ?? 0).toFixed(1)
}

/**
 * Returns the CSS class for a GPU temperature value.
 * Above 80°C is considered dangerously hot.
 */
export function gpuTempClass(tempC: number): string {
  return tempC > 80 ? 'text-red-400' : 'text-white'
}

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
