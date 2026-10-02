import type { MemoryEntry } from '../../types/memory'

const NEVER_USED = 'Never used'
const SEPARATOR = ' · '

const plural = (n: number, one: string, many: string): string => `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`

/** True when no run has been sent this fact and no search has returned it. */
export const isMemoryUnused = (entry: MemoryEntry): boolean => !entry.injected_count && !entry.searched_count

/**
 * One line saying how much a fact has been used: the runs it was sent in
 * (always-on facts) and how often memory_search returned it.
 */
export const formatMemoryUsage = (entry: MemoryEntry): string => {
  if (isMemoryUnused(entry)) return NEVER_USED
  const parts: string[] = []
  if (entry.injected_count) parts.push(`Sent in ${plural(entry.injected_count, 'run', 'runs')}`)
  if (entry.searched_count) parts.push(`Found by search ${plural(entry.searched_count, 'time', 'times')}`)
  return parts.join(SEPARATOR)
}
