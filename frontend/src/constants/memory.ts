import type { MemoryKeep, MemoryPriority, MemoryRecall, MemoryScope, MemoryType, NotesScope } from '../types/memory'

// Display names for memory types (MemoryPanel, MemoryDetail).
export const MEMORY_TYPE_LABEL: Record<MemoryType, string> = {
  long_term: 'Permanent',
  daily: 'Daily',
  session: 'Session',
  user_profile: 'User profile',
}

// The tag that puts a fact in every run's prompt (backend memory.HotTag).
export const HOT_TAG = 'hot'

// User-wide facts are stored under this reserved workspace and follow the
// operator into every workspace (backend globalMemoryWorkspace).
export const GLOBAL_MEMORY_WORKSPACE = 'global'

// Plain-word choices for adding a fact.
export const MEMORY_SCOPE_LABEL: Record<MemoryScope, string> = {
  workspace: 'This workspace',
  user: 'Me, in every workspace',
}
export const MEMORY_RECALL_LABEL: Record<MemoryRecall, string> = {
  on_demand: 'Only when it comes up (searchable)',
  always: 'In every run (always in context)',
}
export const MEMORY_KEEP_LABEL: Record<MemoryKeep, string> = {
  permanent: 'Keep it',
  session: 'This conversation only',
}

// Backend maxMemoryContentChars: a hot fact rides in every prompt, so keep it short.
export const MEMORY_CONTENT_MAX = 2000

// Operator notes (MEMORY.md): what each scope means, in the operator's words.
export const NOTES_SCOPE_LABEL: Record<NotesScope, string> = {
  workspace: 'This workspace',
  global: 'All workspaces',
}

// Priority decides which always-on facts survive when a small context window cuts
// the tail: higher is kept longer.
export const PRIORITY_LOW: MemoryPriority = 0
export const PRIORITY_NORMAL: MemoryPriority = 1
export const PRIORITY_HIGH: MemoryPriority = 2
export const MEMORY_PRIORITY_LABEL: Record<MemoryPriority, string> = {
  0: 'Low — cut first',
  1: 'Normal',
  2: 'High — cut last',
}
