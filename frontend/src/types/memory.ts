// 0 = cut first, 1 = normal, 2 = cut last.
export type MemoryPriority = 0 | 1 | 2

// Server-side views of the list: the always-sent facts, or those never used.
export type MemoryView = 'hot' | 'unused'

export type MemoryType = 'long_term' | 'daily' | 'session' | 'user_profile'

export interface MemoryEntry {
  id: number
  workspace_id: string
  memory_type: MemoryType
  title: string
  content: string
  // "hot" marks a fact that is sent with every run. Defensive about null: older
  // rows can serialise without tags.
  tags?: string[] | null
  source: string
  // How long the fact survives when the budget cuts the tail (0 low, 1 normal, 2 high).
  priority?: MemoryPriority
  // Usage counters: runs the fact was sent in, times search returned it, and when
  // it was last used ('' = never). Counting began when tracking was added.
  injected_count?: number
  searched_count?: number
  last_used_at?: string | null
  created_at: string
  updated_at: string
}

// A fact the model proposed from a chat (POST …/memory-review). `duplicate` marks one memory already holds.
export interface MemorySuggestion {
  content: string
  scope: 'workspace' | 'user'
  mode: 'on_demand' | 'always'
  duplicate: boolean
}

// A suggestion in the review dialog: whether the operator ticked it, and whether saving it just failed.
export interface ReviewItem extends MemorySuggestion {
  selected: boolean
  failed: boolean
}

// The three plain-word choices when adding a fact (mirror the agent's
// memory_update scope / mode / keep).
export type MemoryScope = 'workspace' | 'user'
export type MemoryRecall = 'on_demand' | 'always'
export type MemoryKeep = 'permanent' | 'session'

export interface NewMemory {
  content: string
  scope: MemoryScope
  mode: MemoryRecall
  keep: MemoryKeep
  // Only sent for always-on facts, and only when not the default.
  priority?: MemoryPriority
}

export interface MemoryPreviewEntry {
  id: number
  title: string
  chars: number
  priority: MemoryPriority
}

// What a run of the chosen model would receive (GET …/injection-preview).
export interface MemoryInjectionPreview {
  block: string
  chars: number
  tokens_estimate: number
  budget_chars: number
  // The model's own context budget; 0 when no model was chosen or none resolved.
  context_budget_chars: number
  model: string
  budget_resolved: boolean
  // Size of the operator notes inside the block; over_budget is true when the
  // block exceeds the model's memory budget (notes are never cut, facts give way).
  operator_chars: number
  over_budget: boolean
  included: MemoryPreviewEntry[]
  cut: MemoryPreviewEntry[]
}

// The operator's own MEMORY.md: standing notes injected first in every run.
export type NotesScope = 'workspace' | 'global'

export interface OperatorNotes {
  global: string
  workspace: string
  max_chars: number
}

// What a markdown import did (POST …/import): facts added, facts already saved,
// and entries that could not be used, each with the line of its heading.
export interface MemoryImportIssue {
  line: number
  message: string
}

export interface MemoryImportResult {
  created: number
  skipped: number
  issues: MemoryImportIssue[]
}
