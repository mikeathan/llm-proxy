export type MemoryType = 'long_term' | 'daily' | 'session' | 'user_profile'

export interface MemoryEntry {
  id: number
  workspace_id: string
  memory_type: MemoryType
  title: string
  content: string
  source: string
  created_at: string
  updated_at: string
}
