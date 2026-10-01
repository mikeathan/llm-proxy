import type { MemoryType } from '../types/memory'

// Display names for memory types (MemoryPanel, MemoryDetail).
export const MEMORY_TYPE_LABEL: Record<MemoryType, string> = {
  long_term: 'Permanent',
  daily: 'Daily',
  session: 'Session',
  user_profile: 'User profile',
}
