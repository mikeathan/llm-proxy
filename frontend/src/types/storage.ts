/** One kind of store under the data folder (GET /admin/api/system/storage). `described` is false for an entry no kind names: it is still listed, under its own name, so nothing hides. */
export interface StorageRow {
  id: string
  title: string
  /** "core", a stack's id, or "people" for the workspace folders. */
  owner: string
  entries: string[]
  bytes: number
  changed?: string
  /** Where it can be removed, in fixed words; absent when the page offers no way. */
  clean?: string
  described: boolean
  /** From a stack's own report: the most it lets the file reach, how many records it holds, the oldest one, and how long it keeps one (nanoseconds). */
  cap_bytes?: number
  records?: number
  oldest?: string
  keep_for?: number
}

/** The data management report: the rows under the data folder, the workspace folders (size only) and the volume's free space. */
export interface StorageReport {
  root: string
  rows: StorageRow[]
  total: number
  workspaces?: StorageRow
  free: number
  size: number
  level: 'ok' | 'low' | 'critical' | 'unknown'
}
