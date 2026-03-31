/**
 * Converts an args array to a single space-separated string for display in a text input.
 * @example argsToString(['--ctx-size', '4096']) → "--ctx-size 4096"
 */
export function argsToString(args: string[] | undefined | null): string {
  return (args ?? []).join(' ')
}

/**
 * Parses a space-separated args string back into an array, filtering empty tokens.
 * @example stringToArgs('--ctx-size 4096 ') → ['--ctx-size', '4096']
 */
export function stringToArgs(str: string): string[] {
  return str.split(' ').filter(a => a.trim() !== '')
}

/**
 * Serializes a KEY=VALUE environment map to a newline-separated string for textarea display.
 * @example envMapToString({ FOO: 'bar' }) → "FOO=bar"
 */
export function envMapToString(env: Record<string, string> | undefined | null): string {
  return Object.entries(env ?? {})
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')
}

/**
 * Parses a newline-separated KEY=VALUE string back into a Record.
 * Lines without '=' or with an empty key are silently ignored.
 * The value may itself contain '=' characters (only the first '=' is treated as separator).
 * @example stringToEnvMap("FOO=bar\nBAZ=qux") → { FOO: 'bar', BAZ: 'qux' }
 */
export function stringToEnvMap(str: string): Record<string, string> {
  const env: Record<string, string> = {}
  for (const line of str.split('\n')) {
    const idx = line.indexOf('=')
    if (idx <= 0) continue
    const key = line.slice(0, idx).trim()
    const value = line.slice(idx + 1).trim()
    if (key) env[key] = value
  }
  return env
}
