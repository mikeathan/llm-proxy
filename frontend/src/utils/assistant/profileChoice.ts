import type { ChoiceOption } from '../../types/ui'
import type { ProfileKind, PromptProfile } from '../../types/profile'

const DEFAULT_SUFFIX = ' (default)'
const WORKSPACE_DEFAULT_SUFFIX = ' (this workspace)'

/**
 * The profile a run of this kind starts with: the workspace's own choice when it names a profile that still exists,
 * else the profile that is the kind's default. Undefined only while the catalog is not loaded.
 */
export function defaultProfile(profiles: PromptProfile[], kind: ProfileKind, workspaceChoice = ''): PromptProfile | undefined {
  return profiles.find((p) => p.id === workspaceChoice) ?? profiles.find((p) => p.default_for.includes(kind))
}

/**
 * How a saved profile choice reads: its name, "Default (name)" when none was chosen, and the bare id for one that no
 * longer exists (a deleted profile file), so the page never shows a blank.
 */
export function profileLabel(profiles: PromptProfile[], kind: ProfileKind, id = ''): string {
  if (id) return profiles.find((p) => p.id === id)?.name ?? id
  const start = defaultProfile(profiles, kind)
  return start ? `Default (${start.name})` : 'Default'
}

/** The picker's options, one per profile; the one a run starts with is marked. */
export function profileOptions(profiles: PromptProfile[], kind: ProfileKind, workspaceChoice = ''): ChoiceOption[] {
  const start = defaultProfile(profiles, kind, workspaceChoice)
  const suffix = start && workspaceChoice === start.id ? WORKSPACE_DEFAULT_SUFFIX : DEFAULT_SUFFIX
  return profiles.map((p) => ({ value: p.id, label: p.id === start?.id ? `${p.name}${suffix}` : p.name }))
}
