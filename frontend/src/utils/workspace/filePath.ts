const SEPARATOR = '/'

/**
 * The one encoder for a workspace-relative file path in an API URL: each
 * segment is percent-encoded and the separators are kept, so nested paths
 * reach the server's {file...} wildcard intact (plan Phase 3). Route builders
 * do not use it — vue-router encodes route params itself.
 */
export function encodeFilePath(path: string): string {
  return path.split(SEPARATOR).map(encodeURIComponent).join(SEPARATOR)
}

const PARENT_SEGMENT = '..'
const CURRENT_SEGMENT = '.'

/**
 * Whether `path` is a plain workspace-relative file path: no leading slash or
 * backslash, no empty, "." or ".." segments. The server contains every path
 * anyway (os.Root); this gives the user a clear message first.
 */
export function isSafeRelativePath(path: string): boolean {
  if (!path || path.startsWith(SEPARATOR) || path.includes('\\')) return false
  return path.split(SEPARATOR).every((segment) => segment !== '' && segment !== CURRENT_SEGMENT && segment !== PARENT_SEGMENT)
}
