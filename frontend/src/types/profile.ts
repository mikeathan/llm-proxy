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
  /** Runs stop unless the model is local, so nothing the run sees goes to a cloud provider. */
  local_only?: boolean
  /** Helper profiles this one may hand tasks to (sub-agents); empty or absent means it never delegates. */
  delegates?: string[]
  default_for: ProfileKind[]
}

/** One profile with what the editor needs beyond the picker's entry (backend GET /admin/api/profiles/{id}). */
export interface ProfileDetail extends PromptProfile {
  instructions: string
  /** The profiles that name this one as a helper. */
  delegating_to: string[]
  /** The user's own file that has a built-in's id; deleting it brings the built-in back. */
  replaces_builtin: boolean
}

/** What the editor saves (backend PUT /admin/api/profiles/{id}); the id is the path. */
export interface ProfileInput {
  name: string
  summary: string
  instructions: string
  answer_first: boolean
  may_ask: boolean
  max_steps: number
  tools: string[]
  memory: MemoryMode
  memory_save: PromptProfile['memory_save']
  reminders: boolean
  local_only: boolean
  delegates: string[]
}

export type ProfileFieldKind = 'text' | 'longtext' | 'toggle' | 'number' | 'choice' | 'choices' | 'profiles'

export interface ProfileFieldOption {
  value: string
  label: string
}

/** One setting of a profile as the server describes it (GET /admin/api/profile-schema): the form is drawn from this. */
export interface ProfileField {
  key: keyof ProfileInput
  label: string
  kind: ProfileFieldKind
  help: string
  required?: boolean
  max?: number
  options?: ProfileFieldOption[]
}

/** What a delete left behind. */
export interface ProfileDeleted {
  restored_builtin: boolean
  delegating_to: string[]
}
