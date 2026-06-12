export type MemoryType = 'long_term' | 'daily' | 'session'

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
