import type { MemoryMode } from './automation'

// What a run is for; picks the default profile when nothing chose one. Mirrors the backend profile.Kind.
export type ProfileKind = 'assistant' | 'automation'

// A prompt profile as the picker needs it (backend GET /admin/api/profiles): what it is and what it is the default
// for. The instructions text stays on the server.
export interface PromptProfile {
  id: string
  name: string
  summary: string
  source: 'builtin' | 'user'
  answer_first: boolean
  may_ask: boolean
  max_steps?: number
  tools?: string[]
  memory?: MemoryMode
  memory_save: 'model' | 'explicit' | 'off'
  reminders: boolean
  default_for: ProfileKind[]
}
